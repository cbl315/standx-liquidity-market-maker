package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PriceHandler 价格处理器接口
type PriceHandler interface {
	OnPriceUpdate(data PriceData)
}

// OrderHandler 订单处理器接口
type OrderHandler interface {
	OnOrderUpdate(data OrderDetail)
}

// Client WebSocket 客户端
type Client struct {
	conn         *websocket.Conn
	url          string
	priceHandler PriceHandler
	orderHandler OrderHandler
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	connected    bool
	dialer       *websocket.Dialer
	reconnectCh  chan struct{}
	token        string // JWT token
}

// NewClient 创建 WebSocket 客户端
func NewClient(url string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		url:         url,
		ctx:         ctx,
		cancel:      cancel,
		dialer:      websocket.DefaultDialer,
		reconnectCh: make(chan struct{}, 1),
	}
}

// Connect 连接 WebSocket
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	conn, _, err := c.dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	// 设置 Ping 处理器（服务器每 10s 发送 Ping）
	conn.SetPingHandler(func(appData string) error {
		slog.Debug("received ping from server")
		// 自动回复 Pong（大多数 WebSocket 库会自动处理）
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second*10))
	})

	// 设置 Pong 处理器
	conn.SetPongHandler(func(appData string) error {
		slog.Debug("received pong from server")
		return nil
	})

	c.conn = conn
	c.connected = true

	// 启动消息读取
	c.wg.Add(1)
	go c.readMessages()

	slog.Info("websocket connected", "url", c.url)

	return nil
}

// SetPriceHandler 设置价格处理器
func (c *Client) SetPriceHandler(handler PriceHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.priceHandler = handler
}

// SetOrderHandler 设置订单处理器
func (c *Client) SetOrderHandler(handler OrderHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orderHandler = handler
}

// SetToken 设置 JWT token（用于订阅 order channel）
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// SubscribePrice 订阅价格
func (c *Client) SubscribePrice(symbol string) error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "price",
			Symbol:  symbol,
		},
	}
	return c.sendMessage(req)
}

// AuthOrderChannel 认证订单 channel（使用 JWT token）
func (c *Client) AuthOrderChannel() error {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()

	if token == "" {
		return fmt.Errorf("jwt token not set, call SetToken first")
	}

	req := AuthRequest{
		Auth: AuthParams{
			Token:   token,
			Streams: []StreamSpec{{Channel: "order"}},
		},
	}
	return c.sendMessage(req)
}

// SubscribeOrder 订阅订单（需要先认证）
func (c *Client) SubscribeOrder() error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "order",
		},
	}
	return c.sendMessage(req)
}

// sendMessage 发送消息
func (c *Client) sendMessage(msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("websocket not connected")
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	slog.Debug("sending websocket message", "message", string(msgJSON))

	if err := c.conn.WriteMessage(websocket.TextMessage, msgJSON); err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}

	return nil
}

// readMessages 读取消息
func (c *Client) readMessages() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// 60 秒读取超时
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			messageType, msgJSON, err := c.conn.ReadMessage()
			if err != nil {
				slog.Error("websocket receive failed", "error", err)
				c.triggerReconnect()
				return
			}

			if messageType != websocket.TextMessage {
				slog.Warn("unexpected message type", "type", messageType)
				continue
			}

			c.handleMessage(msgJSON)
		}
	}
}

// triggerReconnect 触发重连
func (c *Client) triggerReconnect() {
	c.mu.Lock()
	c.connected = false
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	select {
	case c.reconnectCh <- struct{}{}:
	default:
	}
}

// ReconnectChan 获取重连通道
func (c *Client) ReconnectChan() <-chan struct{} {
	return c.reconnectCh
}

// handleMessage 处理消息
func (c *Client) handleMessage(msgJSON []byte) {
	slog.Debug("received message", "raw", string(msgJSON))

	var msg map[string]interface{}
	if err := json.Unmarshal(msgJSON, &msg); err != nil {
		slog.Error("unmarshal message failed", "error", err)
		return
	}

	// 处理频道推送消息
	channel, _ := msg["channel"].(string)
	if channel == "" {
		slog.Warn("message without channel")
		return
	}

	switch channel {
	case "price":
		var priceData PriceData
		if err := json.Unmarshal(msgJSON, &priceData); err != nil {
			slog.Error("unmarshal price data failed", "error", err)
			return
		}

		c.mu.Lock()
		handler := c.priceHandler
		c.mu.Unlock()

		if handler != nil {
			handler.OnPriceUpdate(priceData)
		}

	case "order":
		var orderData OrderData
		if err := json.Unmarshal(msgJSON, &orderData); err != nil {
			slog.Error("unmarshal order data failed", "error", err)
			return
		}

		c.mu.Lock()
		handler := c.orderHandler
		c.mu.Unlock()

		if handler != nil {
			handler.OnOrderUpdate(orderData.Data)
		}

	case "auth":
		var authResp AuthResponse
		if err := json.Unmarshal(msgJSON, &authResp); err != nil {
			slog.Error("unmarshal auth response failed", "error", err)
			return
		}
		if authResp.Data.Code == 0 {
			slog.Info("websocket order channel auth successful")
		} else {
			slog.Error("websocket order channel auth failed",
				"code", authResp.Data.Code,
				"msg", authResp.Data.Msg)
		}

	default:
		slog.Debug("unhandled channel", "channel", channel)
	}
}

// Close 关闭连接
func (c *Client) Close() error {
	c.cancel()
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}
	c.connected = false

	return nil
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MessageHandler 消息处理器接口
type MessageHandler interface {
	OnPriceUpdate(msg PriceMessage)
	OnOrderUpdate(msg OrderUpdateMessage)
	OnPositionUpdate(msg PositionUpdateMessage)
	OnLoginSuccess()
	OnError(code int, message string)
}

// Client WebSocket 客户端
type Client struct {
	conn         *websocket.Conn
	url          string
	token        string
	handler      MessageHandler
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	connected    bool
	dialer       *websocket.Dialer
	lastPingTime int64
}

// NewClient 创建 WebSocket 客户端
func NewClient(url string) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		url:    url,
		ctx:    ctx,
		cancel: cancel,
		dialer: websocket.DefaultDialer,
	}
}

// Connect 连接 WebSocket
func (c *Client) Connect(token string) error {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()

	return c.connect()
}

// connect 内部连接方法
func (c *Client) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	conn, _, err := c.dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	// 设置 Ping 处理器
	conn.SetPingHandler(func(appData string) error {
		slog.Debug("received ping from server")
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

	// 启动心跳 (每 10 秒发送一次 ping)
	c.wg.Add(1)
	go c.heartbeat()

	slog.Info("websocket connected", "url", c.url)

	return nil
}

// SetMessageHandler 设置消息处理器
func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handler = handler
}

// SubscribePrice 订阅价格 (public channel)
func (c *Client) SubscribePrice(symbol string) error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "price",
			Symbol:  symbol,
		},
	}
	return c.sendMessage(req)
}

// SubscribeUserOrders 订阅用户订单 (authenticated channel)
func (c *Client) SubscribeUserOrders() error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "order",
		},
	}
	return c.sendMessage(req)
}

// SubscribeUserPositions 订阅用户仓位 (authenticated channel)
func (c *Client) SubscribeUserPositions() error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "position",
		},
	}
	return c.sendMessage(req)
}

// SubscribeUserBalance 订阅用户余额 (authenticated channel)
func (c *Client) SubscribeUserBalance() error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "balance",
		},
	}
	return c.sendMessage(req)
}

// SubscribePublicTrades 订阅公开成交 (public channel)
func (c *Client) SubscribePublicTrades(symbol string) error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "public_trade",
			Symbol:  symbol,
		},
	}
	return c.sendMessage(req)
}

// SubscribeDepthBook 订阅深度行情 (public channel)
func (c *Client) SubscribeDepthBook(symbol string) error {
	req := SubscribeRequest{
		Subscribe: SubscribeParams{
			Channel: "depth_book",
			Symbol:  symbol,
		},
	}
	return c.sendMessage(req)
}

// Login 登录 WebSocket
func (c *Client) Login() error {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()

	req := LoginRequest{
		Login: LoginParams{
			Token: token,
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

	slog.Info("sending websocket message", "raw", string(msgJSON))

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
			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			messageType, msgJSON, err := c.conn.ReadMessage()
			if err != nil {
				slog.Error("websocket receive failed", "error", err)
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

// handleMessage 处理消息
func (c *Client) handleMessage(msgJSON []byte) {
	// 记录原始消息 (调试用)
	slog.Info("received message", "raw", string(msgJSON))

	var msg map[string]interface{}
	if err := json.Unmarshal(msgJSON, &msg); err != nil {
		slog.Error("unmarshal message failed", "error", err)
		return
	}

	// 检查是否为响应消息 (有 code 和 message 字段)
	if code, ok := msg["code"].(float64); ok {
		method, _ := msg["method"].(string)
		message, _ := msg["message"].(string)

		slog.Info("received response", "method", method, "code", int(code), "message", message)

		if method == "login" && int(code) == 0 {
			slog.Info("login successful")
			if c.handler != nil {
				c.handler.OnLoginSuccess()
			}
		} else if int(code) != 0 && c.handler != nil {
			c.handler.OnError(int(code), message)
		}
		return
	}

	// 处理频道推送消息
	channel, _ := msg["channel"].(string)
	if channel == "" {
		slog.Warn("message without channel", "message", string(msgJSON))
		return
	}

	slog.Info("channel message", "channel", channel)

	switch channel {
	case "price":
		var channelMsg ChannelMessage
		if err := json.Unmarshal(msgJSON, &channelMsg); err != nil {
			slog.Error("unmarshal price channel message failed", "error", err)
			return
		}
		var priceData PriceData
		if err := json.Unmarshal(channelMsg.Data, &priceData); err != nil {
			slog.Error("unmarshal price data failed", "error", err)
			return
		}
		// 解析价格
		price, _ := strconv.ParseFloat(priceData.LastPrice, 64)
		priceMsg := PriceMessage{
			Symbol:    priceData.Symbol,
			Price:     price,
			Timestamp: time.Now().UnixMilli(),
		}
		if c.handler != nil {
			c.handler.OnPriceUpdate(priceMsg)
		}

	case "order":
		var channelMsg ChannelMessage
		if err := json.Unmarshal(msgJSON, &channelMsg); err != nil {
			slog.Error("unmarshal order channel message failed", "error", err)
			return
		}
		var orderMsg OrderUpdateMessage
		if err := json.Unmarshal(channelMsg.Data, &orderMsg); err != nil {
			slog.Error("unmarshal order data failed", "error", err)
			return
		}
		if c.handler != nil {
			c.handler.OnOrderUpdate(orderMsg)
		}

	case "position":
		var channelMsg ChannelMessage
		if err := json.Unmarshal(msgJSON, &channelMsg); err != nil {
			slog.Error("unmarshal position channel message failed", "error", err)
			return
		}
		var posMsg PositionUpdateMessage
		if err := json.Unmarshal(channelMsg.Data, &posMsg); err != nil {
			slog.Error("unmarshal position data failed", "error", err)
			return
		}
		if c.handler != nil {
			c.handler.OnPositionUpdate(posMsg)
		}

	case "balance":
		var balanceMsg BalanceUpdateMessage
		dataJSON, _ := json.Marshal(msg["data"])
		if err := json.Unmarshal(dataJSON, &balanceMsg); err != nil {
			slog.Error("unmarshal balance data failed", "error", err)
			return
		}
		slog.Info("balance update", "currency", balanceMsg.Currency, "balance", balanceMsg.Balance)

	default:
		slog.Debug("unknown channel", "channel", channel)
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
		c.connected = false
	}

	return nil
}

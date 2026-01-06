package order

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// Manager 订单管理器
type Manager struct {
	client       *client.Client
	bidOrder     *TrackedOrder
	askOrder     *TrackedOrder
	mutex        sync.RWMutex
	symbol       string
	orderSize    float64
}

// NewManager 创建订单管理器
func NewManager(apiClient *client.Client, symbol string, orderSize float64) *Manager {
	return &Manager{
		client:    apiClient,
		symbol:    symbol,
		orderSize: orderSize,
	}
}

// PlaceBidAsk 下双边订单
func (m *Manager) PlaceBidAsk(bidPrice, askPrice float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.Background()

	// 下 bid 订单
	if err := m.placeBidOrder(ctx, bidPrice); err != nil {
		return fmt.Errorf("place bid order failed: %w", err)
	}

	// 下 ask 订单
	if err := m.placeAskOrder(ctx, askPrice); err != nil {
		return fmt.Errorf("place ask order failed: %w", err)
	}

	slog.Info("placed bid/ask orders",
		"bid_price", bidPrice,
		"ask_price", askPrice,
		"size", m.orderSize)

	return nil
}

// UpdateOrders 更新订单价格
func (m *Manager) UpdateOrders(newBidPrice, newAskPrice float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.Background()

	// 更新 bid 订单
	if m.bidOrder != nil && m.bidOrder.State == OrderStateActive {
		// 取消旧订单
		if err := m.cancelBidOrder(ctx); err != nil {
			slog.Error("cancel bid order failed", "error", err)
		}
	}
	// 下新 bid 订单
	if err := m.placeBidOrder(ctx, newBidPrice); err != nil {
		return err
	}

	// 更新 ask 订单
	if m.askOrder != nil && m.askOrder.State == OrderStateActive {
		// 取消旧订单
		if err := m.cancelAskOrder(ctx); err != nil {
			slog.Error("cancel ask order failed", "error", err)
		}
	}
	// 下新 ask 订单
	if err := m.placeAskOrder(ctx, newAskPrice); err != nil {
		return err
	}

	return nil
}

// CancelAll 取消所有订单
func (m *Manager) CancelAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.Background()

	if m.bidOrder != nil && m.bidOrder.State == OrderStateActive {
		if err := m.cancelBidOrder(ctx); err != nil {
			slog.Error("cancel bid order failed", "error", err)
		}
	}

	if m.askOrder != nil && m.askOrder.State == OrderStateActive {
		if err := m.cancelAskOrder(ctx); err != nil {
			slog.Error("cancel ask order failed", "error", err)
		}
	}

	return nil
}

// GetActiveOrders 获取当前活跃订单
func (m *Manager) GetActiveOrders() *OrderPair {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var pair OrderPair

	if m.bidOrder != nil && m.bidOrder.State == OrderStateActive {
		pair.Bid = m.bidOrder.Order
	}

	if m.askOrder != nil && m.askOrder.State == OrderStateActive {
		pair.Ask = m.askOrder.Order
	}

	return &pair
}

// placeBidOrder 下 bid 订单
func (m *Manager) placeBidOrder(ctx context.Context, price float64) error {
	req := &client.NewOrderRequest{
		Symbol:      m.symbol,
		Side:        client.OrderBid,
		OrderType:   client.OrderTypeLimit,
		Qty:         client.FormatQty(m.orderSize),
		Price:       client.FormatPrice(price),
		TimeInForce: client.TimeInForceGTC,
		ReduceOnly:  false,
	}

	resp, err := m.client.NewOrder(req)
	if err != nil {
		return err
	}

	m.bidOrder = &TrackedOrder{
		Order: &client.Order{
			OrderID: resp.OrderID,
			Symbol:  m.symbol,
			Side:    client.OrderBid,
			Price:   price,
			Qty:     m.orderSize,
			Status:  "open",
		},
		State:      OrderStateActive,
		UpdateTime: time.Now(),
	}

	return nil
}

// placeAskOrder 下 ask 订单
func (m *Manager) placeAskOrder(ctx context.Context, price float64) error {
	req := &client.NewOrderRequest{
		Symbol:      m.symbol,
		Side:        client.OrderAsk,
		OrderType:   client.OrderTypeLimit,
		Qty:         client.FormatQty(m.orderSize),
		Price:       client.FormatPrice(price),
		TimeInForce: client.TimeInForceGTC,
		ReduceOnly:  false,
	}

	resp, err := m.client.NewOrder(req)
	if err != nil {
		return err
	}

	m.askOrder = &TrackedOrder{
		Order: &client.Order{
			OrderID: resp.OrderID,
			Symbol:  m.symbol,
			Side:    client.OrderAsk,
			Price:   price,
			Qty:     m.orderSize,
			Status:  "open",
		},
		State:      OrderStateActive,
		UpdateTime: time.Now(),
	}

	return nil
}

// cancelBidOrder 取消 bid 订单
func (m *Manager) cancelBidOrder(ctx context.Context) error {
	if m.bidOrder == nil {
		return nil
	}

	err := m.client.CancelOrder(m.bidOrder.Order.OrderID)
	if err != nil {
		return err
	}

	m.bidOrder.State = OrderStateIdle
	m.bidOrder = nil

	return nil
}

// cancelAskOrder 取消 ask 订单
func (m *Manager) cancelAskOrder(ctx context.Context) error {
	if m.askOrder == nil {
		return nil
	}

	err := m.client.CancelOrder(m.askOrder.Order.OrderID)
	if err != nil {
		return err
	}

	m.askOrder.State = OrderStateIdle
	m.askOrder = nil

	return nil
}

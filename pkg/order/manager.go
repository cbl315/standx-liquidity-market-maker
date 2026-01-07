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
	client   *client.Client
	bidOrder *TrackedOrder
	askOrder *TrackedOrder
	mutex    sync.RWMutex
	symbol   string
	orderQty float64
	slTpBPS  int // SL/TP 价格浮动范围 (basis points)
}

// NewManager 创建订单管理器
func NewManager(apiClient *client.Client, symbol string, orderQty float64, slTpBPS int) *Manager {
	return &Manager{
		client:   apiClient,
		symbol:   symbol,
		orderQty: orderQty,
		slTpBPS:  slTpBPS,
	}
}

// PlaceBidAsk 下双边订单（带 SL/TP）
func (m *Manager) PlaceBidAsk(bidPrice, askPrice float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.Background()

	// 下 bid 订单（带 SL/TP）
	if err := m.placeBidOrder(ctx, bidPrice); err != nil {
		return fmt.Errorf("place bid order failed: %w", err)
	}

	// 下 ask 订单（带 SL/TP）
	if err := m.placeAskOrder(ctx, askPrice); err != nil {
		return fmt.Errorf("place ask order failed: %w", err)
	}

	slog.Info("placed bid/ask orders with SL/TP",
		"bid_price", bidPrice,
		"ask_price", askPrice,
		"qty", m.orderQty,
		"sl_tp_bps", m.slTpBPS)

	return nil
}

// UpdateOrders 更新订单价格
func (m *Manager) UpdateOrders(newBidPrice, newAskPrice float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ctx := context.Background()

	// 更新 bid 订单
	if m.bidOrder != nil && m.bidOrder.State == OrderStateActive {
		if err := m.cancelBidOrder(ctx); err != nil {
			slog.Error("cancel bid order failed", "error", err)
		}
	}
	if err := m.placeBidOrder(ctx, newBidPrice); err != nil {
		return err
	}

	// 更新 ask 订单
	if m.askOrder != nil && m.askOrder.State == OrderStateActive {
		if err := m.cancelAskOrder(ctx); err != nil {
			slog.Error("cancel ask order failed", "error", err)
		}
	}
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

// calculateSlTpPrices 计算 SL/TP 价格
// bp: basis points (1 bp = 0.01%)
func calculateSlTpPrices(price float64, side client.OrderSide, bp int) (slPrice, tpPrice float64) {
	// bp 转 小数: 2bp = 0.0002
	offset := price * float64(bp) / 10000.0

	if side == client.OrderBid {
		// Bid 单（买入）：止损更低，止盈更高
		slPrice = price - offset
		tpPrice = price + offset
	} else {
		// Ask 单（卖出）：止损更高，止盈更低
		slPrice = price + offset
		tpPrice = price - offset
	}

	return slPrice, tpPrice
}

// placeBidOrder 下 bid 订单（带 SL/TP）
func (m *Manager) placeBidOrder(ctx context.Context, price float64) error {
	// 计算 SL/TP 价格
	slPrice, tpPrice := calculateSlTpPrices(price, client.OrderBid, m.slTpBPS)

	req := &client.NewOrderRequest{
		Symbol:      m.symbol,
		Side:        client.OrderBid,
		OrderType:   client.OrderTypeLimit,
		Qty:         client.FormatQty(m.orderQty),
		Price:       client.FormatPrice(price),
		SlPrice:     client.FormatPrice(slPrice),
		TpPrice:     client.FormatPrice(tpPrice),
		TimeInForce: client.TimeInForceGTC,
		ReduceOnly:  false,
		// ClOrdID 不传，由平台自动生成
	}

	resp, err := m.client.NewOrder(req)
	if err != nil {
		return err
	}

	m.bidOrder = &TrackedOrder{
		Order: &client.Order{
			ID:      0, // 由 API 返回
			Symbol:  m.symbol,
			Side:    client.OrderBid,
			Price:   client.FormatPrice(price),
			Qty:     client.FormatQty(m.orderQty),
			Status:  "open",
			ClOrdID: "", // 将通过 WebSocket 接收
		},
		State:      OrderStateActive,
		UpdateTime: time.Now(),
	}

	slog.Info("bid order placed",
		"order_id", resp.OrderID(),
		"order resp", resp,
		"price", price,
		"sl_price", slPrice,
		"tp_price", tpPrice,
		"qty", m.orderQty,
		"cl_ord_id", "pending from websocket")

	return nil
}

// placeAskOrder 下 ask 订单（带 SL/TP）
func (m *Manager) placeAskOrder(ctx context.Context, price float64) error {
	// 计算 SL/TP 价格
	slPrice, tpPrice := calculateSlTpPrices(price, client.OrderAsk, m.slTpBPS)

	req := &client.NewOrderRequest{
		Symbol:      m.symbol,
		Side:        client.OrderAsk,
		OrderType:   client.OrderTypeLimit,
		Qty:         client.FormatQty(m.orderQty),
		Price:       client.FormatPrice(price),
		SlPrice:     client.FormatPrice(slPrice),
		TpPrice:     client.FormatPrice(tpPrice),
		TimeInForce: client.TimeInForceGTC,
		ReduceOnly:  false,
		// ClOrdID 不传，由平台自动生成
	}

	resp, err := m.client.NewOrder(req)
	if err != nil {
		return err
	}

	m.askOrder = &TrackedOrder{
		Order: &client.Order{
			ID:      0, // 由 API 返回
			Symbol:  m.symbol,
			Side:    client.OrderAsk,
			Price:   client.FormatPrice(price),
			Qty:     client.FormatQty(m.orderQty),
			Status:  "open",
			ClOrdID: "", // 将通过 WebSocket 接收
		},
		State:      OrderStateActive,
		UpdateTime: time.Now(),
	}

	slog.Info("ask order placed",
		"order_id", resp.OrderID(),
		"price", price,
		"sl_price", slPrice,
		"tp_price", tpPrice,
		"qty", m.orderQty,
		"cl_ord_id", "pending from websocket")

	return nil
}

// cancelBidOrder 取消 bid 订单
func (m *Manager) cancelBidOrder(ctx context.Context) error {
	if m.bidOrder == nil {
		return nil
	}

	// 查询当前 open 订单，找到 bid 订单的 cl_ord_id
	orders, err := m.client.GetOpenOrdersByStatus(m.symbol, "open")
	if err != nil {
		return fmt.Errorf("query open orders failed: %w", err)
	}

	// 找到 buy side 的订单
	var clOrdID string
	for _, ord := range orders {
		if ord.Side == "buy" {
			clOrdID = ord.ClOrdID
			break
		}
	}

	if clOrdID == "" {
		slog.Warn("no bid order found in open orders")
		m.bidOrder = nil
		return nil
	}

	slog.Info("cancelling bid order", "cl_ord_id", clOrdID)

	err = m.client.CancelOrder(clOrdID)
	if err != nil {
		return err
	}

	slog.Info("bid order cancelled", "cl_ord_id", clOrdID)

	m.bidOrder.State = OrderStateIdle
	m.bidOrder = nil

	return nil
}

// cancelAskOrder 取消 ask 订单
func (m *Manager) cancelAskOrder(ctx context.Context) error {
	if m.askOrder == nil {
		return nil
	}

	// 查询当前 open 订单，找到 ask 订单的 cl_ord_id
	orders, err := m.client.GetOpenOrdersByStatus(m.symbol, "open")
	if err != nil {
		return fmt.Errorf("query open orders failed: %w", err)
	}

	// 找到 sell side 的订单
	var clOrdID string
	for _, ord := range orders {
		if ord.Side == "sell" {
			clOrdID = ord.ClOrdID
			break
		}
	}

	if clOrdID == "" {
		slog.Warn("no ask order found in open orders")
		m.askOrder = nil
		return nil
	}

	slog.Info("cancelling ask order", "cl_ord_id", clOrdID)

	err = m.client.CancelOrder(clOrdID)
	if err != nil {
		return err
	}

	slog.Info("ask order cancelled", "cl_ord_id", clOrdID)

	m.askOrder.State = OrderStateIdle
	m.askOrder = nil

	return nil
}

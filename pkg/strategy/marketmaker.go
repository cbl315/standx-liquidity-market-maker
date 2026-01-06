package strategy

import (
	"context"
	"log/slog"
	"math"
	"sync"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/order"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/risk"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

// MarketMaker 做市策略
type MarketMaker struct {
	orderMgr   *order.Manager
	riskMgr    *risk.Manager
	wsClient   *ws.Client
	symbol     string
	orderSize  float64
	spreadBPS  int
	mu         sync.RWMutex
	currentBid float64
	currentAsk float64
	isRunning  bool
}

// NewMarketMaker 创建做市策略
func NewMarketMaker(
	orderMgr *order.Manager,
	riskMgr *risk.Manager,
	wsClient *ws.Client,
	symbol string,
	orderSize float64,
	spreadBPS int,
) *MarketMaker {
	return &MarketMaker{
		orderMgr:  orderMgr,
		riskMgr:   riskMgr,
		wsClient:  wsClient,
		symbol:    symbol,
		orderSize: orderSize,
		spreadBPS: spreadBPS,
	}
}

// Run 运行做市策略
func (mm *MarketMaker) Run(ctx context.Context) error {
	mm.mu.Lock()
	mm.isRunning = true
	mm.mu.Unlock()

	slog.Info("market maker strategy started",
		"symbol", mm.symbol,
		"order_size", mm.orderSize,
		"spread_bps", mm.spreadBPS)

	// 设置消息处理器
	mm.wsClient.SetMessageHandler(mm)

	// 订阅价格和订单
	if err := mm.wsClient.SubscribePrice(mm.symbol); err != nil {
		return err
	}
	if err := mm.wsClient.SubscribeUserOrders(); err != nil {
		return err
	}

	// 等待上下文取消
	<-ctx.Done()

	// 清理
	mm.orderMgr.CancelAll()

	slog.Info("market maker strategy stopped")
	return nil
}

// OnPriceUpdate 处理价格更新
func (mm *MarketMaker) OnPriceUpdate(msg ws.PriceMessage) {
	slog.Debug("price update received",
		"symbol", msg.Symbol,
		"price", msg.Price,
		"timestamp", msg.Timestamp)

	// 检查是否紧急停止
	if mm.riskMgr.IsEmergencyStop() {
		slog.Warn("emergency stop active, skipping order update")
		return
	}

	// 计算新价格
	newBid, newAsk := mm.calculatePrices(msg.Price)

	// 检查是否需要更新
	mm.mu.RLock()
	needsUpdate := mm.shouldUpdateOrders(newBid, newAsk)
	mm.mu.RUnlock()

	if needsUpdate {
		if err := mm.orderMgr.UpdateOrders(newBid, newAsk); err != nil {
			slog.Error("update orders failed", "error", err)
		} else {
			mm.mu.Lock()
			mm.currentBid = newBid
			mm.currentAsk = newAsk
			mm.mu.Unlock()

			slog.Info("orders updated",
				"bid", newBid,
				"ask", newAsk,
				"mark_price", msg.Price)
		}
	}
}

// OnOrderUpdate 处理订单更新
func (mm *MarketMaker) OnOrderUpdate(msg ws.OrderUpdateMessage) {
	slog.Info("order update received",
		"order_id", msg.OrderID,
		"status", msg.Status,
		"filled_qty", msg.FilledQty)

	// 检查订单是否成交
	if msg.Status == "filled" {
		slog.Warn("order filled",
			"order_id", msg.OrderID,
			"side", msg.Order.Side,
			"filled_qty", msg.FilledQty)

		// 触发风险管理
		if err := mm.riskMgr.OnOrderFilled(msg.Order); err != nil {
			slog.Error("risk management failed", "error", err)
		}

		// 恢复做市
		mm.restoreOrders()
	}
}

// OnPositionUpdate 处理仓位更新
func (mm *MarketMaker) OnPositionUpdate(msg ws.PositionUpdateMessage) {
	slog.Debug("position update received",
		"symbol", msg.Symbol,
		"size", msg.Size,
		"unrealized_pnl", msg.UnrealizedPnL)
}

// calculatePrices 计算挂单价格
func (mm *MarketMaker) calculatePrices(markPrice float64) (bid, ask float64) {
	// spreadBPS 是基点，1 bps = 0.01%
	spread := float64(mm.spreadBPS) / 10000 / 2 // 分到两边

	// bid = markPrice * (1 - spread)
	// ask = markPrice * (1 + spread)
	bid = markPrice * (1 - spread)
	ask = markPrice * (1 + spread)

	return bid, ask
}

// shouldUpdateOrders 检查是否需要更新订单
func (mm *MarketMaker) shouldUpdateOrders(newBid, newAsk float64) bool {
	// 价格变化超过 1 bps 才更新
	const thresholdBPS = 1
	threshold := thresholdBPS / 10000.0

	bidChange := math.Abs(newBid-mm.currentBid) / mm.currentBid
	askChange := math.Abs(newAsk-mm.currentAsk) / mm.currentAsk

	return bidChange > threshold || askChange > threshold
}

// restoreOrders 恢复做市订单
func (mm *MarketMaker) restoreOrders() {
	// 获取当前市价
	// TODO: 从缓存获取或 API 获取
	// 然后重新下单

	slog.Info("restoring market making orders after fill")
}

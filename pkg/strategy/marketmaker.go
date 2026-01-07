package strategy

import (
	"context"
	"log/slog"
	"math"
	"strconv"
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
	orderQty   float64
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
	orderQty float64,
	spreadBPS int,
) *MarketMaker {
	return &MarketMaker{
		orderMgr:  orderMgr,
		riskMgr:   riskMgr,
		wsClient:  wsClient,
		symbol:    symbol,
		orderQty:  orderQty,
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
		"order_qty", mm.orderQty,
		"spread_bps", mm.spreadBPS)

	// 设置价格处理器
	mm.wsClient.SetPriceHandler(mm)

	// 订阅价格
	if err := mm.wsClient.SubscribePrice(mm.symbol); err != nil {
		return err
	}

	// 等待上下文取消
	<-ctx.Done()

	// 清理
	mm.orderMgr.CancelAll()

	slog.Info("market maker strategy stopped")
	return nil
}

// OnPriceUpdate 处理价格更新（实现 ws.PriceHandler 接口）
func (mm *MarketMaker) OnPriceUpdate(data ws.PriceData) {
	slog.Debug("price update received",
		"symbol", data.Symbol,
		"mark_price", data.Data.MarkPrice,
		"seq", data.Seq)

	// 解析标记价格
	markPrice, err := strconv.ParseFloat(data.Data.MarkPrice, 64)
	if err != nil {
		slog.Error("parse mark price failed", "error", err)
		return
	}

	// 检查余额是否足够
	hasEnough, err := mm.riskMgr.HasSufficientBalance(mm.orderQty)
	if err != nil {
		slog.Error("balance check failed", "error", err)
		return
	}

	if !hasEnough {
		slog.Debug("insufficient balance, waiting for SL/TP to close positions")
		return
	}

	// 计算新价格
	newBid, newAsk := mm.calculatePrices(markPrice)

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
				"mark_price", markPrice)
		}
	}
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
	// 首次下单
	if mm.currentBid == 0 || mm.currentAsk == 0 {
		return true
	}

	// 价格变化超过 1 bps 才更新
	const thresholdBPS = 1
	threshold := thresholdBPS / 10000.0

	bidChange := math.Abs(newBid-mm.currentBid) / mm.currentBid
	askChange := math.Abs(newAsk-mm.currentAsk) / mm.currentAsk

	return bidChange > threshold || askChange > threshold
}

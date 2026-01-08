package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/order"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/risk"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/timewindow"
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
	paused     bool  // 是否已暂停（窗口期控制）
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
	// 检查是否暂停（窗口期）
	mm.mu.RLock()
	if mm.paused {
		mm.mu.RUnlock()
		return
	}
	mm.mu.RUnlock()

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

// Pause 暂停做市（用于窗口期控制）
func (mm *MarketMaker) Pause() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.paused {
		return
	}

	mm.paused = true
	slog.Info("market maker paused (time window)")
}

// Resume 恢复做市
func (mm *MarketMaker) Resume() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if !mm.paused {
		return
	}

	mm.paused = false
	slog.Info("market maker resumed (time window ended)")
}

// IsPaused 检查是否已暂停
func (mm *MarketMaker) IsPaused() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.paused
}

// monitorTimeWindow 运行时监控时间窗口（在独立 goroutine 中运行）
func (mm *MarketMaker) MonitorTimeWindow(
	ctx context.Context,
	windowFilter *timewindow.Filter,
	apiClient *client.Client,
	orderMgr *order.Manager,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 启动时立即检查一次当前状态
	wasInWindow := windowFilter.IsInWindow()
	if wasInWindow {
		slog.Warn("started in time window, pausing market making immediately",
			"window_end", windowFilter.GetNextWindowEnd().Format("2006-01-02 15:04:05 MST"))
		mm.Pause()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			isInWindow := windowFilter.IsInWindow()

			// 检测状态变化
			if isInWindow && !wasInWindow {
				// 进入窗口期
				slog.Warn("entering time window, pausing market making",
					"window_end", windowFilter.GetNextWindowEnd().Format("2006-01-02 15:04:05 MST"))

				// 1. 暂停做市
				mm.Pause()

				// 2. 取消所有订单
				slog.Info("canceling all orders...")
				orders, err := apiClient.GetOpenOrders(mm.symbol)
				if err != nil {
					slog.Error("get open orders failed", "error", err)
				} else {
					for _, ord := range orders {
						if err := apiClient.CancelOrder(ord.ClOrdID); err != nil {
							slog.Error("cancel order failed", "cl_ord_id", ord.ClOrdID, "error", err)
						} else {
							slog.Info("order canceled", "cl_ord_id", ord.ClOrdID)
						}
					}
				}

				// 3. 平掉所有仓位
				slog.Info("closing positions...")
				pos, err := apiClient.GetPosition(mm.symbol)
				if err != nil {
					slog.Error("get position failed", "error", err)
				} else {
					size, err := parsePositionSize(pos.Qty)
					if err != nil {
						slog.Error("parse position size failed", "error", err)
					} else if size != 0 {
						var side client.OrderSide = client.OrderAsk
						if size < 0 {
							side = client.OrderBid
						}
						closeReq := &client.NewOrderRequest{
							Symbol:      mm.symbol,
							Side:        side,
							OrderType:   client.OrderTypeMarket,
							Qty:         formatPositionSize(abs(size)),
							TimeInForce: client.TimeInForceIOC,
							ReduceOnly:  true,
						}
						if _, err := apiClient.NewOrder(closeReq); err != nil {
							slog.Error("close position failed", "error", err)
						} else {
							slog.Info("position closed", "size", size)
						}
					}
				}

				wasInWindow = true

			} else if !isInWindow && wasInWindow {
				// 离开窗口期，恢复做市
				slog.Info("time window ended, resuming market making")
				mm.Resume()
				wasInWindow = false
			}
		}
	}
}

// parsePositionSize 解析仓位大小字符串为 float64
func parsePositionSize(qtyStr string) (float64, error) {
	var qty float64
	_, err := fmt.Sscanf(qtyStr, "%f", &qty)
	return qty, err
}

// formatPositionSize 格式化仓位大小为字符串
func formatPositionSize(size float64) string {
	return fmt.Sprintf("%.6f", size)
}

// abs 返回浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

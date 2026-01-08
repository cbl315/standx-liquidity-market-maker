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
	"github.com/cbl315/standx-liquidity-market-maker/pkg/volatility"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

// PauseState 暂停状态
type PauseState int

const (
	PauseStateNone            PauseState = iota // 未暂停
	PauseStateTimeWindow                        // 时间窗口暂停
	PauseStateHighVolatility                    // 高波动暂停
	PauseStatePositionCooldown                  // 仓位冷却暂停
)

func (s PauseState) String() string {
	switch s {
	case PauseStateNone:
		return "none"
	case PauseStateTimeWindow:
		return "time_window"
	case PauseStateHighVolatility:
		return "high_volatility"
	case PauseStatePositionCooldown:
		return "position_cooldown"
	default:
		return "unknown"
	}
}

func (s PauseState) IsPaused() bool {
	return s != PauseStateNone
}

// MarketMaker 做市策略
type MarketMaker struct {
	orderMgr   *order.Manager
	riskMgr    *risk.Manager
	wsClient   *ws.Client
	volGuard   *volatility.Guard // 波动保护器
	symbol     string
	orderQty   float64
	spreadBPS  int
	mu         sync.RWMutex
	currentBid float64
	currentAsk float64
	isRunning  bool
	pauseState PauseState  // 暂停状态
}

// NewMarketMaker 创建做市策略
func NewMarketMaker(
	orderMgr *order.Manager,
	riskMgr *risk.Manager,
	wsClient *ws.Client,
	volGuard *volatility.Guard,
	symbol string,
	orderQty float64,
	spreadBPS int,
) *MarketMaker {
	return &MarketMaker{
		orderMgr:  orderMgr,
		riskMgr:   riskMgr,
		wsClient:  wsClient,
		volGuard:  volGuard,
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
	// 检查是否暂停（任何原因）
	if mm.IsPaused() {
		return
	}

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

	// 检查波动率（如果启用）
	if mm.volGuard != nil && mm.volGuard.Enabled() {
		shouldPause, volatilityBPS, msg := mm.volGuard.ShouldPause(markPrice)
		if shouldPause {
			slog.Debug("market making paused due to high volatility",
				"volatility_bps", volatilityBPS,
				"reason", msg)
			return
		}
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

// Pause 暂停做市
// apiClient: API 客户端，用于取消订单和平仓
// state: 暂停状态（PauseStateTimeWindow 或 PauseStateHighVolatility）
//
// 暂停优先级：TimeWindow > HighVolatility
// 如果当前是高波动暂停，但请求窗口期暂停，会升级为窗口期暂停
func (mm *MarketMaker) Pause(apiClient *client.Client, state PauseState) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 检查是否需要更新状态
	shouldUpdate := false
	var oldState PauseState

	if mm.pauseState == PauseStateNone {
		// 当前未暂停，总是更新
		shouldUpdate = true
		oldState = mm.pauseState
	} else if mm.pauseState == PauseStateHighVolatility && state == PauseStateTimeWindow {
		// 从高波动升级到窗口期（窗口期优先级更高）
		shouldUpdate = true
		oldState = mm.pauseState
		slog.Info("upgrading pause state", "from", mm.pauseState.String(), "to", state.String())
	} else {
		// 已经处于某种暂停状态，不需要更新
		slog.Debug("already paused, skipping", "current_state", mm.pauseState, "new_state", state)
		return
	}

	if shouldUpdate {
		mm.pauseState = state
		slog.Warn("market maker paused", "state", state.String(), "previous_state", oldState.String())

		// 取消所有订单并平仓（在解锁状态下执行，避免死锁）
		mm.mu.Unlock()
		mm.cancelAllAndClosePositions(apiClient, state.String())
		mm.mu.Lock()
	}
}

// Resume 恢复做市
// state: 要恢复的状态（只有当前状态匹配时才会恢复）
func (mm *MarketMaker) Resume(state PauseState) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.pauseState != state {
		// 当前状态不匹配，说明是其他原因导致的暂停，不处理
		slog.Debug("resume state mismatch, skipping", "current_state", mm.pauseState, "expected_state", state)
		return
	}

	mm.pauseState = PauseStateNone
	slog.Info("market maker resumed", "previous_state", state.String())
}

// IsPaused 检查是否已暂停
func (mm *MarketMaker) IsPaused() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.pauseState.IsPaused()
}

// GetPauseState 获取当前暂停状态
func (mm *MarketMaker) GetPauseState() PauseState {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.pauseState
}

// cancelAllAndClosePositions 取消所有订单并平仓
// reason: 触发原因（用于日志记录），如 "time window" 或 "high volatility"
func (mm *MarketMaker) cancelAllAndClosePositions(apiClient *client.Client, reason string) {
	// 1. 取消所有订单
	slog.Info("canceling all orders...", "reason", reason)
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

	// 2. 平掉所有仓位
	slog.Info("closing positions...", "reason", reason)
	pos, err := apiClient.GetPosition(mm.symbol)
	if err != nil {
		slog.Error("get position failed", "error", err)
		return
	}

	size, err := parsePositionSize(pos.Qty)
	if err != nil {
		slog.Error("parse position size failed", "error", err)
		return
	}

	if size == 0 {
		slog.Info("no open position to close")
		return
	}

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
		slog.Info("position closed", "size", size, "reason", reason)
	}
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
		mm.Pause(apiClient, PauseStateTimeWindow)
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

				// 暂停做市（Pause 方法内部会取消订单并平仓）
				mm.Pause(apiClient, PauseStateTimeWindow)

				wasInWindow = true

			} else if !isInWindow && wasInWindow {
				// 离开窗口期，恢复做市
				slog.Info("time window ended, resuming market making")
				mm.Resume(PauseStateTimeWindow)
				wasInWindow = false
			}
		}
	}
}

// MonitorVolatility 运行时监控波动率（在独立 goroutine 中运行）
func (mm *MarketMaker) MonitorVolatility(
	ctx context.Context,
	apiClient *client.Client,
	orderMgr *order.Manager,
) {
	// 如果没有启用波动保护，直接返回
	if mm.volGuard == nil || !mm.volGuard.Enabled() {
		return
	}

	ticker := time.NewTicker(5 * time.Second) // 每 5 秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// 如果已经因时间窗口暂停，跳过波动率监控
			if mm.GetPauseState() == PauseStateTimeWindow {
				continue
			}

			// 检查波动状态变化
			_, justEntered, justExited := mm.volGuard.CheckStateChange()

			if justEntered {
				// 刚进入高波动状态
				slog.Warn("high volatility detected, pausing market making and closing positions")

				// 暂停做市（Pause 方法内部会取消订单并平仓）
				mm.Pause(apiClient, PauseStateHighVolatility)

			} else if justExited {
				// 波动恢复正常，尝试恢复做市
				// 注意：如果当前已经因为窗口期暂停，Resume 会返回而不执行
				currentState := mm.GetPauseState()
				if currentState == PauseStateHighVolatility {
					slog.Info("volatility normalized, resuming market making")
					mm.Resume(PauseStateHighVolatility)
				} else {
					slog.Debug("volatility normalized but cannot resume due to other pause reason",
						"current_state", currentState.String())
				}
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

// MonitorPositions 监控仓位并自动平仓（在独立 goroutine 中运行）
// 吃单后马上平单，然后进入冷静期暂停开单一段时间
func (mm *MarketMaker) MonitorPositions(
	ctx context.Context,
	apiClient *client.Client,
	checkInterval time.Duration,
	cooldownDuration time.Duration,
) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 用于跟踪是否在冷却期
	var cooldownTimer *time.Timer
	var inCooldown bool

	for {
		select {
		case <-ctx.Done():
			if cooldownTimer != nil {
				cooldownTimer.Stop()
			}
			return

		case <-ticker.C:
			// 如果在冷却期，跳过检查
			if inCooldown {
				continue
			}

			// 查询仓位
			positions, err := apiClient.QueryPositions(mm.symbol)
			if err != nil {
				slog.Error("query positions failed", "error", err)
				continue
			}

			// 检查是否有开放的仓位
			hasOpenPosition := false
			for _, pos := range positions {
				if pos.Status == "open" {
					size, err := parsePositionSize(pos.Qty)
					if err != nil {
						slog.Error("parse position size failed", "error", err)
						continue
					}
					if size != 0 {
						hasOpenPosition = true
						break
					}
				}
			}

			if !hasOpenPosition {
				// 没有开放仓位，继续监控
				continue
			}

			// 有开放仓位，调用 Pause 会自动取消所有订单并平仓
			slog.Warn("open position detected, entering cooldown and closing positions",
				"symbol", mm.symbol,
				"cooldown_duration", cooldownDuration)

			mm.Pause(apiClient, PauseStatePositionCooldown)
			inCooldown = true

			// 设置冷却定时器
			cooldownTimer = time.AfterFunc(cooldownDuration, func() {
				slog.Info("position cooldown ended, resuming market making")
				mm.Resume(PauseStatePositionCooldown)
				inCooldown = false
			})
		}
	}
}

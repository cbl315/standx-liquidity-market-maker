package risk

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// Manager 风险管理器
type Manager struct {
	client          *client.Client
	config          RiskConfig
	positionTracker *PositionTracker
	symbol          string
	pnlHistory      []PnLRecord
	emergencyStop   bool
}

// NewManager 创建风险管理器
func NewManager(apiClient *client.Client, symbol string, config RiskConfig) *Manager {
	return &Manager{
		client: apiClient,
		config: config,
		positionTracker: &PositionTracker{
			NetPosition: 0,
			AvgPrice:    0,
			RealizedPnL: 0,
			DailyPnL:    0,
		},
		symbol:     symbol,
		pnlHistory: make([]PnLRecord, 0),
	}
}

// OnOrderFilled 处理订单成交（立即平仓策略）
func (rm *Manager) OnOrderFilled(filledOrder *client.Order) error {
	if !rm.config.Enabled {
		slog.Info("risk management disabled, skipping position close")
		return nil
	}

	slog.Warn("order filled, executing immediate close strategy",
		"order_id", filledOrder.OrderID,
		"side", filledOrder.Side,
		"size", filledOrder.Qty,
		"price", filledOrder.Price)

	// 1. 获取当前市价
	marketPrice, err := rm.client.GetMarkPrice(rm.symbol)
	if err != nil {
		return fmt.Errorf("get market price failed: %w", err)
	}

	// 2. 计算盈亏
	pnl := rm.CalculatePnL(filledOrder.Price, marketPrice, filledOrder.Side, filledOrder.Qty)

	slog.Info("calculated pnl for filled order",
		"entry_price", filledOrder.Price,
		"market_price", marketPrice,
		"pnl", pnl,
		"pnl_percent", pnl/marketPrice*100)

	// 3. 检查风险限制
	if err := rm.CheckRiskLimits(pnl); err != nil {
		slog.Error("risk limit exceeded", "error", err)
		rm.emergencyStop = true
		return err
	}

	// 4. 立即市价平仓
	if rm.config.AutoClosePosition {
		if err := rm.ClosePositionAtMarket(filledOrder.Side, filledOrder.Qty); err != nil {
			slog.Error("close position at market failed", "error", err)
			return err
		}
	}

	// 5. 记录盈亏
	rm.recordPnL(PnLRecord{
		Timestamp:  time.Now(),
		Side:       filledOrder.Side,
		Size:       filledOrder.Qty,
		EntryPrice: filledOrder.Price,
		ExitPrice:  marketPrice,
		PnL:        pnl,
	})

	return nil
}

// ClosePositionAtMarket 立即市价平仓
func (rm *Manager) ClosePositionAtMarket(side client.OrderSide, size float64) error {
	// bid 被吃 = 持有 BTC，需要 ask 平仓
	// ask 被吃 = 持有空头，需要 bid 平仓
	var closeSide client.OrderSide
	if side == client.OrderBid {
		closeSide = client.OrderAsk
		slog.Info("closing long position at market", "size", size)
	} else {
		closeSide = client.OrderBid
		slog.Info("closing short position at market", "size", size)
	}

	req := &client.NewOrderRequest{
		Symbol:      rm.symbol,
		Side:        closeSide,
		OrderType:   client.OrderTypeMarket,
		Qty:         client.FormatQty(size),
		TimeInForce: client.TimeInForceIOC,
		ReduceOnly:  true,
	}

	_, cancel := context.WithTimeout(context.Background(), rm.config.CloseTimeout)
	defer cancel()

	// 重试机制
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := rm.client.NewOrder(req)
		if err != nil {
			lastErr = err
			slog.Warn("close order failed, retrying", "attempt", i+1, "error", err)
			time.Sleep(time.Second)
			continue
		}

		slog.Info("position closed successfully",
			"order_id", resp.OrderID,
			"side", closeSide,
			"size", size)
		return nil
	}

	return fmt.Errorf("close position failed after %d retries: %w", maxRetries, lastErr)
}

// CalculatePnL 计算盈亏
func (rm *Manager) CalculatePnL(entryPrice, exitPrice float64, side client.OrderSide, size float64) float64 {
	var pnl float64
	if side == client.OrderBid {
		// bid 被吃：买入 BTC，盈亏 = (exitPrice - entryPrice) * size
		pnl = (exitPrice - entryPrice) * size
	} else {
		// ask 被吃：卖出 BTC，盈亏 = (entryPrice - exitPrice) * size
		pnl = (entryPrice - exitPrice) * size
	}
	return pnl
}

// CheckRiskLimits 检查风险限制
func (rm *Manager) CheckRiskLimits(currentPnL float64) error {
	rm.positionTracker.mutex.Lock()
	defer rm.positionTracker.mutex.Unlock()

	// 更新当日盈亏
	rm.positionTracker.DailyPnL += currentPnL

	// 检查单笔亏损限制
	if currentPnL < 0 {
		pnlPercent := math.Abs(currentPnL) / rm.positionTracker.AvgPrice
		if pnlPercent > rm.config.MaxLossPerTrade {
			return fmt.Errorf("single trade loss exceeded: %.4f%% > %.4f%%",
				pnlPercent*100, rm.config.MaxLossPerTrade*100)
		}
	}

	// 检查日累计亏损限制
	if rm.positionTracker.DailyPnL < 0 {
		dailyLossPercent := math.Abs(rm.positionTracker.DailyPnL) / rm.positionTracker.AvgPrice
		if dailyLossPercent > rm.config.DailyLossLimit {
			return fmt.Errorf("daily loss limit exceeded: %.4f%% > %.4f%%",
				dailyLossPercent*100, rm.config.DailyLossLimit*100)
		}
	}

	return nil
}

// recordPnL 记录盈亏
func (rm *Manager) recordPnL(record PnLRecord) {
	rm.positionTracker.mutex.Lock()
	defer rm.positionTracker.mutex.Unlock()

	rm.pnlHistory = append(rm.pnlHistory, record)

	// 保留最近 1000 条记录
	if len(rm.pnlHistory) > 1000 {
		rm.pnlHistory = rm.pnlHistory[1:]
	}

	// 更新平均价格
	if rm.positionTracker.NetPosition == 0 {
		rm.positionTracker.AvgPrice = record.ExitPrice
	} else {
		rm.positionTracker.AvgPrice = (rm.positionTracker.AvgPrice + record.EntryPrice) / 2
	}
}

// GetDailyPnL 获取当日盈亏
func (rm *Manager) GetDailyPnL() float64 {
	rm.positionTracker.mutex.RLock()
	defer rm.positionTracker.mutex.RUnlock()
	return rm.positionTracker.DailyPnL
}

// IsEmergencyStop 检查是否紧急停止
func (rm *Manager) IsEmergencyStop() bool {
	return rm.emergencyStop
}

// ResetDailyPnL 重置日盈亏（每日零点调用）
func (rm *Manager) ResetDailyPnL() {
	rm.positionTracker.mutex.Lock()
	defer rm.positionTracker.mutex.Unlock()

	rm.positionTracker.DailyPnL = 0
	slog.Info("daily pnl reset")
}

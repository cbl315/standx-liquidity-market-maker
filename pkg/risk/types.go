package risk

import (
	"sync"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// RiskConfig 风险配置
type RiskConfig struct {
	Enabled           bool          `json:"enabled"`
	Strategy          string        `json:"strategy"`
	MaxLossPerTrade   float64       `json:"max_loss_per_trade"`
	DailyLossLimit    float64       `json:"daily_loss_limit"`
	AutoClosePosition bool          `json:"auto_close_position"`
	CloseTimeout      time.Duration `json:"close_timeout"`
}

// PositionTracker 仓位追踪
type PositionTracker struct {
	NetPosition float64 `json:"net_position"` // 净仓位 (BTC)
	AvgPrice    float64 `json:"avg_price"`    // 平均成交价
	RealizedPnL float64 `json:"realized_pnl"` // 已实现盈亏
	DailyPnL    float64 `json:"daily_pnl"`     // 当日累计盈亏
	mutex       sync.RWMutex
}

// PnLRecord 盈亏记录
type PnLRecord struct {
	Timestamp  time.Time         `json:"timestamp"`
	Side       client.OrderSide  `json:"side"`
	Size       float64           `json:"size"`
	EntryPrice float64           `json:"entry_price"`
	ExitPrice  float64           `json:"exit_price"`
	PnL        float64           `json:"pnl"`
}

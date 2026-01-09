package volatility

import (
	"log/slog"
	"math"
	"sync"
	"time"
)

// PriceSnapshot 价格快照
type PriceSnapshot struct {
	Timestamp time.Time
	Price     float64
}

// Guard 波动保护器
type Guard struct {
	thresholdBPS int                 // 波动阈值（基点），如 50 bps = 0.5%
	windowSec    int                 // 检测窗口时长（秒）
	minSnapshots int                 // 最小快照数量，避免样本太少误判
	snapshots    []PriceSnapshot     // 价格历史快照
	mu           sync.RWMutex        // 保护 snapshots 的并发访问
	enabled      bool                // 是否启用
	isPaused     bool                // 当前是否因高波动暂停
	pausedAt     time.Time           // 暂停开始时间
	wasPaused    bool                // 上次检查时的暂停状态（用于检测状态变化）
}

// NewGuard 创建波动保护器
func NewGuard(thresholdBPS, windowSec, minSnapshots int) *Guard {
	return &Guard{
		thresholdBPS: thresholdBPS,
		windowSec:    windowSec,
		minSnapshots: minSnapshots,
		snapshots:    make([]PriceSnapshot, 0, windowSec*2), // 预分配容量
		enabled:      true,
		wasPaused:    false,
	}
}

// Enabled 返回是否启用
func (g *Guard) Enabled() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.enabled
}

// SetEnabled 设置启用状态
func (g *Guard) SetEnabled(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabled = enabled
	slog.Info("volatility guard enabled state changed", "enabled", enabled)
}

// IsPaused 返回当前是否因高波动暂停
func (g *Guard) IsPaused() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.isPaused
}

// AddPrice 添加价格快照（不触发暂停检查）
// 用于在暂停状态下继续收集价格数据，以便后续能正确恢复
func (g *Guard) AddPrice(currentPrice float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.enabled {
		return
	}

	now := time.Now()
	cutoffTime := now.Add(-time.Duration(g.windowSec) * time.Second)

	// 添加当前价格快照
	g.snapshots = append(g.snapshots, PriceSnapshot{
		Timestamp: now,
		Price:     currentPrice,
	})

	// 移除窗口外的旧数据
	newSnapshots := make([]PriceSnapshot, 0, len(g.snapshots))
	for _, snap := range g.snapshots {
		if snap.Timestamp.After(cutoffTime) {
			newSnapshots = append(newSnapshots, snap)
		}
	}
	g.snapshots = newSnapshots
}

// CheckStateChange 检查暂停状态是否发生变化
// 返回: (当前是否暂停, 是否刚刚进入暂停状态, 是否刚刚恢复)
func (g *Guard) CheckStateChange() (paused, justEntered, justExited bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 重新计算当前波动率（即使处于暂停状态）
	currentPaused := g.recalculatePausedState()

	justEntered = currentPaused && !g.wasPaused
	justExited = !currentPaused && g.wasPaused

	// 更新上次状态
	g.wasPaused = currentPaused

	return currentPaused, justEntered, justExited
}

// recalculatePausedState 基于当前快照重新计算是否应该暂停
func (g *Guard) recalculatePausedState() bool {
	// 样本数量不足，不暂停（也不恢复）
	if len(g.snapshots) < g.minSnapshots {
		return g.isPaused // 保持当前状态
	}

	// 计算当前波动率
	minPrice := g.snapshots[0].Price
	maxPrice := g.snapshots[0].Price
	for _, snap := range g.snapshots {
		if snap.Price < minPrice {
			minPrice = snap.Price
		}
		if snap.Price > maxPrice {
			maxPrice = snap.Price
		}
	}

	avgPrice := (minPrice + maxPrice) / 2
	volatilityBPS := int(math.Round((maxPrice-minPrice)/avgPrice * 10000))

	now := time.Now()

	// 检查是否超过阈值
	if volatilityBPS >= g.thresholdBPS {
		if !g.isPaused {
			g.isPaused = true
			g.pausedAt = now
			slog.Warn("high volatility detected", "volatility_bps", volatilityBPS)
		}
		return true
	}

	// 波动率低于阈值，检查是否可以恢复
	if g.isPaused {
		pausedDuration := now.Sub(g.pausedAt)
		if pausedDuration > time.Duration(g.windowSec)*time.Second {
			g.isPaused = false
			g.pausedAt = time.Time{}
			slog.Info("volatility normalized", "volatility_bps", volatilityBPS)
		}
	}

	return g.isPaused
}

// ShouldPause 检查是否应该暂停做市
// 返回: (是否暂停, 当前波动BPS, 消息)
func (g *Guard) ShouldPause(currentPrice float64) (bool, int, string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.enabled {
		return false, 0, "volatility guard disabled"
	}

	now := time.Now()
	cutoffTime := now.Add(-time.Duration(g.windowSec) * time.Second)

	// 添加当前价格快照
	g.snapshots = append(g.snapshots, PriceSnapshot{
		Timestamp: now,
		Price:     currentPrice,
	})

	// 移除窗口外的旧数据
	newSnapshots := make([]PriceSnapshot, 0, len(g.snapshots))
	for _, snap := range g.snapshots {
		if snap.Timestamp.After(cutoffTime) {
			newSnapshots = append(newSnapshots, snap)
		}
	}
	g.snapshots = newSnapshots

	// 如果样本数量不足，不触发暂停
	if len(g.snapshots) < g.minSnapshots {
		return false, 0, "insufficient data"
	}

	// 计算窗口内的最高价和最低价
	minPrice := g.snapshots[0].Price
	maxPrice := g.snapshots[0].Price
	for _, snap := range g.snapshots {
		if snap.Price < minPrice {
			minPrice = snap.Price
		}
		if snap.Price > maxPrice {
			maxPrice = snap.Price
		}
	}

	// 计算波动率（以基点为单位）
	// 波动率 = (max - min) / avg * 10000
	avgPrice := (minPrice + maxPrice) / 2
	volatilityBPS := int(math.Round((maxPrice-minPrice)/avgPrice * 10000))

	// 检查是否超过阈值
	if volatilityBPS >= g.thresholdBPS {
		// 如果已经在暂停状态，保持暂停
		if !g.isPaused {
			g.isPaused = true
			g.pausedAt = now
			slog.Warn("high volatility detected, pausing market making",
				"volatility_bps", volatilityBPS,
				"threshold_bps", g.thresholdBPS,
				"window_seconds", g.windowSec,
				"min_price", minPrice,
				"max_price", maxPrice)
		}
		return true, volatilityBPS, "high volatility detected"
	}

	// 如果当前波动率低于阈值，且之前处于暂停状态，则恢复
	if g.isPaused {
		// 需要持续一段时间稳定才能恢复，避免频繁切换
		pausedDuration := now.Sub(g.pausedAt)
		if pausedDuration > time.Duration(g.windowSec)*time.Second {
			g.isPaused = false
			g.pausedAt = time.Time{}
			slog.Info("volatility normalized, resuming market making",
				"volatility_bps", volatilityBPS,
				"threshold_bps", g.thresholdBPS,
				"paused_duration", pausedDuration)
		}
	}

	return g.isPaused, volatilityBPS, "normal"
}

// GetVolatilityStats 获取当前波动统计信息
func (g *Guard) GetVolatilityStats() (currentBPS int, isPaused bool, snapshotCount int) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.snapshots) < 2 {
		return 0, g.isPaused, len(g.snapshots)
	}

	minPrice := g.snapshots[0].Price
	maxPrice := g.snapshots[0].Price
	for _, snap := range g.snapshots {
		if snap.Price < minPrice {
			minPrice = snap.Price
		}
		if snap.Price > maxPrice {
			maxPrice = snap.Price
		}
	}

	avgPrice := (minPrice + maxPrice) / 2
	currentBPS = int(math.Round((maxPrice-minPrice)/avgPrice * 10000))

	return currentBPS, g.isPaused, len(g.snapshots)
}

// Clear 清除历史数据（用于测试或重置）
func (g *Guard) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.snapshots = make([]PriceSnapshot, 0, g.windowSec*2)
	g.isPaused = false
	g.pausedAt = time.Time{}
	slog.Info("volatility guard cleared")
}

// ForceResume 强制恢复（用于手动恢复）
func (g *Guard) ForceResume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.isPaused {
		g.isPaused = false
		g.pausedAt = time.Time{}
		slog.Info("volatility guard force resumed")
	}
}

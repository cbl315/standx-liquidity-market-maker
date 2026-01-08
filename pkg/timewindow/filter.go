package timewindow

import (
	"log/slog"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/config"
)

// WindowStatus 时间窗口状态
type WindowStatus string

const (
	StatusInWindow      WindowStatus = "in_window"       // 在窗口期内
	StatusBeforeWindow  WindowStatus = "before_window"   // 窗口期即将开始
	StatusOutsideWindow WindowStatus = "outside_window"  // 在窗口期外
	StatusDisabled      WindowStatus = "disabled"        // 功能未启用
)

// Filter 时间窗口过滤器
type Filter struct {
	cfg    config.TimeWindowConfig
	loc    *time.Location
}

// NewFilter 创建时间窗口过滤器
func NewFilter(cfg config.TimeWindowConfig) (*Filter, error) {
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return nil, err
	}

	return &Filter{
		cfg: cfg,
		loc: loc,
	}, nil
}

// ShouldRun 判断当前时间是否可以运行
// 返回值:
//   - bool: true 表示可以运行, false 表示在窗口期内
//   - time.Duration: 距离窗口期开始/结束的剩余时间
//   - WindowStatus: 当前状态
func (f *Filter) ShouldRun() (bool, time.Duration, WindowStatus) {
	if !f.cfg.Enabled {
		return true, 0, StatusDisabled
	}

	now := time.Now().In(f.loc)
	weekday := int(now.Weekday()) // Sunday=0, Monday=1, ..., Saturday=6

	// 转换为配置格式 (Monday=1, ..., Sunday=7)
	if weekday == 0 {
		weekday = 7
	}

	// 检查每个时间窗口
	for _, window := range f.cfg.Windows {
		// 检查是否在指定的星期几
		if !f.inWeekdays(weekday, window.Weekdays) {
			continue
		}

		// 解析时间
		startTime, err := time.Parse("15:04", window.Start)
		if err != nil {
			slog.Error("parse window start time failed", "error", err, "start", window.Start)
			continue
		}

		endTime, err := time.Parse("15:04", window.End)
		if err != nil {
			slog.Error("parse window end time failed", "error", err, "end", window.End)
			continue
		}

		// 构建今天的开始和结束时间
		windowStart := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, f.loc)
		windowEnd := time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, f.loc)

		// 处理跨天情况
		if endTime.Before(startTime) {
			// 窗口跨天，例如 22:00 - 03:00
			if now.Hour() > startTime.Hour() || (now.Hour() == startTime.Hour() && now.Minute() >= startTime.Minute()) {
				// 当前时间在开始时间之后（当天），窗口结束时间是第二天
				windowEnd = windowEnd.Add(24 * time.Hour)
			} else {
				// 当前时间在结束时间之前（第二天），窗口开始时间是前一天
				windowStart = windowStart.Add(-24 * time.Hour)
			}
		}

		// 检查是否在窗口内
		if now.After(windowStart) && now.Before(windowEnd) {
			// 在窗口期内
			remaining := windowEnd.Sub(now)
			return false, remaining, StatusInWindow
		}

		// 检查是否在窗口期之前（即将进入窗口期）
		if now.Before(windowStart) {
			remaining := windowStart.Sub(now)
			if remaining < 24*time.Hour {
				return false, remaining, StatusBeforeWindow
			}
		}
	}

	return true, 0, StatusOutsideWindow
}

// IsInWindow 判断当前时间是否在窗口期内
func (f *Filter) IsInWindow() bool {
	shouldRun, _, status := f.ShouldRun()
	return !shouldRun && status == "in_window"
}

// GetNextWindowEnd 获取下一个窗口期结束时间
func (f *Filter) GetNextWindowEnd() time.Time {
	now := time.Now().In(f.loc)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}

	for _, window := range f.cfg.Windows {
		if !f.inWeekdays(weekday, window.Weekdays) {
			continue
		}

		startTime, _ := time.Parse("15:04", window.Start)
		endTime, _ := time.Parse("15:04", window.End)

		windowStart := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, f.loc)
		windowEnd := time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, f.loc)

		if endTime.Before(startTime) {
			if now.Hour() > startTime.Hour() || (now.Hour() == startTime.Hour() && now.Minute() >= startTime.Minute()) {
				windowEnd = windowEnd.Add(24 * time.Hour)
			} else {
				windowStart = windowStart.Add(-24 * time.Hour)
			}
		}

		if now.After(windowStart) && now.Before(windowEnd) {
			return windowEnd
		}
	}

	return now
}

// inWeekdays 检查当前星期是否在窗口期配置的星期列表中
func (f *Filter) inWeekdays(current int, weekdays []int) bool {
	for _, wd := range weekdays {
		if wd == current {
			return true
		}
	}
	return false
}

// WaitForWindowEnd 等待窗口期结束
func (f *Filter) WaitForWindowEnd() <-chan time.Time {
	ch := make(chan time.Time, 1)

	go func() {
		for {
			shouldRun, duration, status := f.ShouldRun()
			if shouldRun {
				ch <- time.Now().In(f.loc)
				return
			}

			if status == "in_window" {
				// 在窗口期内，等待窗口期结束
				slog.Info("in time window, waiting for window end",
					"window_end", f.GetNextWindowEnd().Format("2006-01-02 15:04:05"),
					"wait_duration", duration)

				time.Sleep(duration)
			} else if status == "before_window" {
				// 在窗口期之前，等待窗口期开始
				slog.Info("before time window, will enter window soon",
					"wait_duration", duration)

				time.Sleep(duration)
				// 进入窗口期后继续等待结束
				continue
			} else {
				// 不在任何窗口期，直接返回
				ch <- time.Now().In(f.loc)
				return
			}
		}
	}()

	return ch
}

// GetAction 获取窗口期内执行的操作
func (f *Filter) GetAction() config.WindowAction {
	return f.cfg.Action
}

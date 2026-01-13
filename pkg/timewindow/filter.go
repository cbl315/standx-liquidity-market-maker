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
//
// 跨天窗口处理逻辑：
// 使用"周内总分钟数"方法统一处理跨天情况。
// 例如：weekdays=[1,2,3,4,5], start=21:00, end=06:00
// - 周五 21:00 开启的窗口会持续到周六 06:00，不管周六是否在 weekdays 中
// - 窗口由其开始时间对应的 weekday 决定是否生效，而不是结束时间
func (f *Filter) ShouldRun() (bool, time.Duration, WindowStatus) {
	if !f.cfg.Enabled {
		return true, 0, StatusDisabled
	}

	now := time.Now().In(f.loc)
	nowWeekday := int(now.Weekday()) // Sunday=0, Monday=1, ..., Saturday=6

	// 转换为配置格式 (Monday=1, ..., Sunday=7)
	if nowWeekday == 0 {
		nowWeekday = 7
	}

	// 检查每个时间窗口
	for _, window := range f.cfg.Windows {
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

		// 计算周内分钟数 (周一 00:00 = 0, 周日 23:59 = 10079)
		startMinutes := f.toWeekMinutes(startTime.Hour(), startTime.Minute(), 1) // 默认周一
		endMinutes := f.toWeekMinutes(endTime.Hour(), endTime.Minute(), 1)       // 默认周一

		// 处理跨天：如果结束时间小于开始时间，说明跨天
		if endMinutes <= startMinutes {
			endMinutes += 10080 // 加一周的分钟数
		}

		// 检查每个可能的开始日期（窗口可能在今天、昨天或前天开启）
		// 最多检查前3天，因为窗口最长跨2天
		for dayOffset := 0; dayOffset <= 2; dayOffset++ {
			windowWeekday := nowWeekday - dayOffset
			if windowWeekday <= 0 {
				windowWeekday += 7
			}

			// 检查窗口开始日是否在 weekdays 配置中
			if !f.inWeekdays(windowWeekday, window.Weekdays) {
				continue
			}

			// 计算这个窗口开始日对应的分钟数
			windowStartMinutes := f.toWeekMinutes(startTime.Hour(), startTime.Minute(), windowWeekday)
			windowEndMinutes := windowStartMinutes + (endMinutes - startMinutes)

			nowMinutes := f.toWeekMinutes(now.Hour(), now.Minute(), nowWeekday)

			// 如果当前时间在窗口开始之前，可能需要加一周的分钟数
			// 这样可以正确处理跨周的情况（虽然不太可能发生）
			for nowMinutes < windowStartMinutes && dayOffset > 0 {
				nowMinutes += 10080
			}

			// 检查是否在窗口内
			if nowMinutes >= windowStartMinutes && nowMinutes < windowEndMinutes {
				remaining := windowEndMinutes - nowMinutes
				return false, time.Duration(remaining) * time.Minute, StatusInWindow
			}

			// 检查是否即将进入窗口期
			if nowMinutes < windowStartMinutes {
				remaining := windowStartMinutes - nowMinutes
				if remaining < 24*60 && dayOffset == 0 {
					// 只有今天即将开始的窗口才返回 before_window
					return false, time.Duration(remaining) * time.Minute, StatusBeforeWindow
				}
			}
		}
	}

	return true, 0, StatusOutsideWindow
}

// toWeekMinutes 将 (hour, minute, weekday) 转换为周内总分钟数
// weekday: 1=周一, 2=周二, ..., 7=周日
// 返回值: 周一 00:00 = 0, 周日 23:59 = 10079
func (f *Filter) toWeekMinutes(hour, minute, weekday int) int {
	// weekday=1 是周一，所以偏移量是 weekday-1
	return (weekday-1)*1440 + hour*60 + minute
}

// IsInWindow 判断当前时间是否在窗口期内
func (f *Filter) IsInWindow() bool {
	shouldRun, _, status := f.ShouldRun()
	return !shouldRun && status == "in_window"
}

// GetNextWindowEnd 获取下一个窗口期结束时间
func (f *Filter) GetNextWindowEnd() time.Time {
	now := time.Now().In(f.loc)
	nowWeekday := int(now.Weekday())
	if nowWeekday == 0 {
		nowWeekday = 7
	}

	for _, window := range f.cfg.Windows {
		startTime, _ := time.Parse("15:04", window.Start)
		endTime, _ := time.Parse("15:04", window.End)

		// 计算周内分钟数
		startMinutes := f.toWeekMinutes(startTime.Hour(), startTime.Minute(), 1)
		endMinutes := f.toWeekMinutes(endTime.Hour(), endTime.Minute(), 1)

		// 处理跨天
		if endMinutes <= startMinutes {
			endMinutes += 10080
		}

		// 检查每个可能的开始日期
		for dayOffset := 0; dayOffset <= 2; dayOffset++ {
			windowWeekday := nowWeekday - dayOffset
			if windowWeekday <= 0 {
				windowWeekday += 7
			}

			if !f.inWeekdays(windowWeekday, window.Weekdays) {
				continue
			}

			windowStartMinutes := f.toWeekMinutes(startTime.Hour(), startTime.Minute(), windowWeekday)
			windowEndMinutes := windowStartMinutes + (endMinutes - startMinutes)

			nowMinutes := f.toWeekMinutes(now.Hour(), now.Minute(), nowWeekday)

			for nowMinutes < windowStartMinutes && dayOffset > 0 {
				nowMinutes += 10080
			}

			if nowMinutes >= windowStartMinutes && nowMinutes < windowEndMinutes {
				// 计算结束时间的具体日期时间
				windowDuration := windowEndMinutes - windowStartMinutes
				windowStart := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, f.loc)

				// 调整到正确的开始日期
				if dayOffset > 0 {
					windowStart = windowStart.AddDate(0, 0, -dayOffset)
				}

				return windowStart.Add(time.Duration(windowDuration) * time.Minute)
			}
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

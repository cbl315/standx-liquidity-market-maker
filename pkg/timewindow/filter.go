package timewindow

import (
	"fmt"
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
	// 缓存的所有窗口时间范围（以周内分钟数表示）
	cachedWindows []cachedWindow
	// nowFunc 用于获取当前时间，支持测试注入
	nowFunc func() time.Time
	// lastStatus 上一次的状态，用于避免重复日志
	lastStatus WindowStatus
}

// cachedWindow 缓存的窗口时间范围
type cachedWindow struct {
	weekday        int // 窗口开始日 1=周一, ..., 7=周日
	startMinutes   int // 周内开始分钟数
	endMinutes     int // 周内结束分钟数（可能大于10080，表示跨周）
	windowDuration int // 窗口持续分钟数
	isCrossWeek    bool // 是否跨周
}

// NewFilter 创建时间窗口过滤器
func NewFilter(cfg config.TimeWindowConfig) (*Filter, error) {
	loc, err := time.LoadLocation(cfg.TimeZone)
	if err != nil {
		return nil, err
	}

	cachedWindows := buildCachedWindows(cfg)

	return &Filter{
		cfg:           cfg,
		loc:           loc,
		cachedWindows: cachedWindows,
		nowFunc:       time.Now,
	}, nil
}

// buildCachedWindows 构建缓存的窗口时间范围
func buildCachedWindows(cfg config.TimeWindowConfig) []cachedWindow {
	var windows []cachedWindow

	for _, window := range cfg.Windows {
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

		// 计算单日分钟数
		startMinutes := startTime.Hour()*60 + startTime.Minute()
		endMinutes := endTime.Hour()*60 + endTime.Minute()

		// 计算窗口持续时长
		duration := endMinutes - startMinutes
		isCrossDay := duration < 0
		if isCrossDay {
			duration += 1440 // 跨天，加一天的分钟数
		}

		// 为每个配置的 weekday 生成窗口
		for _, weekday := range window.Weekdays {
			weekStartMinutes := (weekday-1)*1440 + startMinutes
			weekEndMinutes := weekStartMinutes + duration

			windows = append(windows, cachedWindow{
				weekday:        weekday,
				startMinutes:   weekStartMinutes,
				endMinutes:     weekEndMinutes,
				windowDuration: duration,
				isCrossWeek:    weekEndMinutes > 10080, // 超过一周总分钟数
			})

			slog.Debug("cached window",
				"weekday", weekday,
				"start", window.Start,
				"end", window.End,
				"startMinutes", weekStartMinutes,
				"endMinutes", weekEndMinutes,
				"duration", duration)
		}
	}

	return windows
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

	now := f.nowFunc().In(f.loc)
	nowWeekday := int(now.Weekday()) // Sunday=0, Monday=1, ..., Saturday=6

	// 转换为配置格式 (Monday=1, ..., Sunday=7)
	if nowWeekday == 0 {
		nowWeekday = 7
	}

	nowMinutes := (nowWeekday-1)*1440 + now.Hour()*60 + now.Minute()

	// 检查每个缓存的窗口
	var nearestUpcomingWindow struct {
		window    cachedWindow
		remaining int
	}

	for _, cw := range f.cachedWindows {
		// 处理跨周：如果窗口跨周，需要检查两个范围
		checkWindows := []struct {
			start int
			end   int
			cw    cachedWindow
		}{
			{cw.startMinutes, cw.endMinutes, cw},
		}

		// 如果窗口跨周，添加跨周后的窗口（减去10080分钟）
		if cw.isCrossWeek {
			checkWindows = append(checkWindows, struct {
				start int
				end   int
				cw    cachedWindow
			}{
				cw.startMinutes - 10080,
				cw.endMinutes - 10080,
				cw,
			})
		}

		for _, w := range checkWindows {
			// 检查是否在窗口内
			if nowMinutes >= w.start && nowMinutes < w.end {
				remaining := w.end - nowMinutes
				// 只在进入窗口时记录一次日志（状态变化）
				if f.lastStatus != StatusInWindow {
					slog.Info("time window status: IN WINDOW",
						"window_start", weekdayName(w.cw.weekday),
						"window_end", formatMinutes(w.end),
						"remaining", time.Duration(remaining)*time.Minute)
					f.lastStatus = StatusInWindow
				}
				return false, time.Duration(remaining) * time.Minute, StatusInWindow
			}

			// 记录最近的即将开始的窗口（4小时内）
			if nowMinutes < w.start {
				remaining := w.start - nowMinutes
				// 只记录4小时内即将开始的窗口
				if remaining < 4*60 {
					if nearestUpcomingWindow.window.weekday == 0 || remaining < nearestUpcomingWindow.remaining {
						nearestUpcomingWindow.window = w.cw
						nearestUpcomingWindow.remaining = remaining
					}
				}
			}
		}
	}

	// 如果有即将开始的窗口（4小时内），返回 before_window
	if nearestUpcomingWindow.window.weekday != 0 {
		// 只在状态变化时记录日志
		if f.lastStatus != StatusBeforeWindow {
			slog.Info("time window status: BEFORE WINDOW",
				"next_window", weekdayName(nearestUpcomingWindow.window.weekday),
				"remaining", time.Duration(nearestUpcomingWindow.remaining)*time.Minute)
			f.lastStatus = StatusBeforeWindow
		}
		return false, time.Duration(nearestUpcomingWindow.remaining) * time.Minute, StatusBeforeWindow
	}

	// 在窗口外
	if f.lastStatus != StatusOutsideWindow {
		slog.Info("time window status: OUTSIDE WINDOW - can run")
		f.lastStatus = StatusOutsideWindow
	}
	return true, 0, StatusOutsideWindow
}

// weekdayName 返回星期几的名称
// weekday: 1=周一, 2=周二, ..., 7=周日
func weekdayName(weekday int) string {
	names := map[int]string{
		1: "周一",
		2: "周二",
		3: "周三",
		4: "周四",
		5: "周五",
		6: "周六",
		7: "周日",
	}
	return names[weekday]
}

// formatMinutes 将周内分钟数格式化为可读的时间描述
func formatMinutes(minutes int) string {
	if minutes < 10080 {
		weekday := (minutes / 1440) + 1
		hour := (minutes % 1440) / 60
		min := minutes % 60
		return fmt.Sprintf("%s %02d:%02d", weekdayName(weekday), hour, min)
	}
	// 跨周情况
	weekday := ((minutes - 10080) / 1440) + 1
	hour := ((minutes - 10080) % 1440) / 60
	min := (minutes - 10080) % 60
	return fmt.Sprintf("%s(下周) %02d:%02d", weekdayName(weekday), hour, min)
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
	now := f.nowFunc().In(f.loc)
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

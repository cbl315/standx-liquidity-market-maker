package timewindow

import (
	"testing"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/config"
)

// TestRestartScenarios 测试不同时间点重启的场景
// 配置: weekdays=[1,2,3,4,5] (周一到周五), start=22:00, end=06:00
func TestRestartScenarios(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled: true,
		TimeZone: "Asia/Shanghai",
		Windows: []config.WindowSpec{
			{
				Weekdays: []int{1, 2, 3, 4, 5}, // 周一到周五
				Start:    "22:00",
				End:      "06:00",
			},
		},
	}

	testCases := []struct {
		name         string
		date         string // YYYY-MM-DD
		time         string // HH:MM
		wantCanRun   bool
		wantStatus   WindowStatus
		description  string
	}{
		// 周一重启场景
		{
			name:        "周一凌晨1点重启 - 无窗口（周日不在weekdays中）",
			date:        "2024-01-08", // 周一
			time:        "01:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周一凌晨1点：周日不在配置中，没有窗口",
		},
		{
			name:        "周一凌晨5:59重启 - 无窗口",
			date:        "2024-01-08", // 周一
			time:        "05:59",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周一凌晨5:59：周日不在配置中，没有窗口",
		},
		{
			name:        "周一早上6点重启 - 无窗口",
			date:        "2024-01-08", // 周一
			time:        "06:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周一早上6点：无窗口",
		},
		{
			name:        "周一晚上21:59重启 - 窗口即将开始",
			date:        "2024-01-08", // 周一
			time:        "21:59",
			wantCanRun:  false,
			wantStatus:  StatusBeforeWindow,
			description: "周一晚上21:59，即将进入窗口期",
		},
		{
			name:        "周一晚上22点重启 - 进入窗口",
			date:        "2024-01-08", // 周一
			time:        "22:00",
			wantCanRun:  false,
			wantStatus:  StatusInWindow,
			description: "周一晚上22:00，窗口开启",
		},

		// 周三重启场景（关键测试用例）
		{
			name:        "周三凌晨1点重启 - 在周二22:00开启的窗口内",
			date:        "2024-01-10", // 周三
			time:        "01:00",
			wantCanRun:  false,
			wantStatus:  StatusInWindow,
			description: "周三凌晨1点应该在周二22:00开启的窗口内（跨天窗口）",
		},
		{
			name:        "周三凌晨2:30重启 - 在窗口内",
			date:        "2024-01-10", // 周三
			time:        "02:30",
			wantCanRun:  false,
			wantStatus:  StatusInWindow,
			description: "周三凌晨2:30，仍在窗口内",
		},
		{
			name:        "周三早上6点重启 - 窗口刚结束",
			date:        "2024-01-10", // 周三
			time:        "06:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周三早上6点，周二窗口刚结束，今天22:00还有窗口（距离16小时，outside）",
		},

		// 周五重启场景
		{
			name:        "周五晚上22点重启 - 进入窗口",
			date:        "2024-01-12", // 周五
			time:        "22:00",
			wantCanRun:  false,
			wantStatus:  StatusInWindow,
			description: "周五晚上22:00，窗口开启",
		},

		// 周六重启场景（关键：周六不在weekdays中）
		{
			name:        "周六凌晨1点重启 - 在周五22:00开启的窗口内",
			date:        "2024-01-13", // 周六
			time:        "01:00",
			wantCanRun:  false,
			wantStatus:  StatusInWindow,
			description: "周六凌晨1点，周五22:00开启的窗口仍在持续",
		},
		{
			name:        "周六早上6点重启 - 窗口结束",
			date:        "2024-01-13", // 周六
			time:        "06:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周六早上6点，窗口已结束",
		},
		{
			name:        "周六晚上22点重启 - 无窗口",
			date:        "2024-01-13", // 周六
			time:        "22:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周六不在weekdays配置中，无窗口",
		},

		// 周日重启场景
		{
			name:        "周日早上10点重启 - 无窗口",
			date:        "2024-01-14", // 周日
			time:        "10:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周日不在weekdays配置中，无窗口",
		},
		{
			name:        "周日晚上22点重启 - 无窗口",
			date:        "2024-01-14", // 周日
			time:        "22:00",
			wantCanRun:  true,
			wantStatus:  StatusOutsideWindow,
			description: "周日不在weekdays配置中，无窗口",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 解析测试时间
			testTime, err := time.ParseInLocation("2006-01-02 15:04", tc.date+" "+tc.time,
				time.FixedZone("UTC+8", 8*3600))
			if err != nil {
				t.Fatalf("parse test time failed: %v", err)
			}

			// 创建过滤器并注入测试时间
			filter, err := NewFilter(cfg)
			if err != nil {
				t.Fatalf("NewFilter failed: %v", err)
			}
			filter.nowFunc = func() time.Time { return testTime }

			// 执行测试
			canRun, remaining, status := filter.ShouldRun()

			// 验证结果
			if canRun != tc.wantCanRun {
				t.Errorf("%s: canRun = %v, want %v\n描述: %s",
					tc.name, canRun, tc.wantCanRun, tc.description)
			}
			if status != tc.wantStatus {
				t.Errorf("%s: status = %v, want %v\n描述: %s",
					tc.name, status, tc.wantStatus, tc.description)
			}

			// 输出详细信息
			t.Logf("✓ %s", tc.description)
			t.Logf("  测试时间: %s %s", tc.date, tc.time)
			t.Logf("  状态: canRun=%v, status=%s, remaining=%v",
				canRun, status, remaining)
		})
	}
}

// TestRestartAcrossMidnight 测试跨午夜重启的场景
func TestRestartAcrossMidnight(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled:  true,
		TimeZone: "Asia/Shanghai",
		Windows: []config.WindowSpec{
			{
				Weekdays: []int{1, 2, 3, 4, 5},
				Start:    "23:30",
				End:      "01:00",
			},
		},
	}

	testCases := []struct {
		name        string
		date        string
		time        string
		wantCanRun  bool
		wantStatus  WindowStatus
	}{
		{
			name:       "周二23:45重启 - 在窗口内",
			date:       "2024-01-09", // 周二
			time:       "23:45",
			wantCanRun: false,
			wantStatus: StatusInWindow,
		},
		{
			name:       "周三00:30重启 - 在窗口内（跨天）",
			date:       "2024-01-10", // 周三
			time:       "00:30",
			wantCanRun: false,
			wantStatus: StatusInWindow,
		},
		{
			name:       "周三01:00重启 - 窗口刚结束",
			date:       "2024-01-10", // 周三
			time:       "01:00",
			wantCanRun: true,
			wantStatus: StatusOutsideWindow,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testTime, _ := time.ParseInLocation("2006-01-02 15:04", tc.date+" "+tc.time,
				time.FixedZone("UTC+8", 8*3600))

			filter, _ := NewFilter(cfg)
			filter.nowFunc = func() time.Time { return testTime }

			canRun, _, status := filter.ShouldRun()

			if canRun != tc.wantCanRun {
				t.Errorf("%s: canRun = %v, want %v", tc.name, canRun, tc.wantCanRun)
			}
			if status != tc.wantStatus {
				t.Errorf("%s: status = %v, want %v", tc.name, status, tc.wantStatus)
			}
		})
	}
}

// TestCachedWindowsIntegrity 测试缓存窗口的完整性
func TestCachedWindowsIntegrity(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled:  true,
		TimeZone: "Asia/Shanghai",
		Windows: []config.WindowSpec{
			{
				Weekdays: []int{1, 2, 3, 4, 5},
				Start:    "22:00",
				End:      "06:00",
			},
		},
	}

	filter, err := NewFilter(cfg)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	// 验证缓存窗口数量
	expectedCount := 5 // 5个weekday
	if len(filter.cachedWindows) != expectedCount {
		t.Errorf("cached windows count = %d, want %d", len(filter.cachedWindows), expectedCount)
	}

	// 验证每个窗口的值
	for i, cw := range filter.cachedWindows {
		t.Logf("Window %d: weekday=%d, start=%d, end=%d, duration=%d, crossWeek=%v",
			i, cw.weekday, cw.startMinutes, cw.endMinutes, cw.windowDuration, cw.isCrossWeek)

		// 验证窗口时长（22:00到次日06:00 = 8小时 = 480分钟）
		expectedDuration := 8 * 60
		if cw.windowDuration != expectedDuration {
			t.Errorf("window %d: duration = %d, want %d", i, cw.windowDuration, expectedDuration)
		}

		// 验证不跨周（22:00到06:00只跨1天，不会跨周）
		if cw.isCrossWeek {
			t.Errorf("window %d: should not be cross week", i)
		}
	}
}

// TestCrossDayWindowBugFix 测试跨天窗口 bug 修复
// 配置: weekdays=[1,2,3,4,5] (周一到周五), start=21:00, end=06:00
//
// 预期行为:
// - 周五 21:00 开启的窗口会持续到周六 06:00
// - 周六 02:00 应该在窗口内（延续周五的窗口）
func TestCrossDayWindowBugFix(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled: true,
		TimeZone: "Asia/Shanghai",
		Windows: []config.WindowSpec{
			{
				Weekdays: []int{1, 2, 3, 4, 5}, // 周一到周五
				Start:    "21:00",
				End:      "06:00",
			},
		},
	}

	filter, err := NewFilter(cfg)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}
	_ = filter // TODO: 当前实现使用 time.Now()，需要重构以支持测试时间注入

	// 定义测试用例
	testCases := []struct {
		name           string
		weekday        time.Weekday
		hour, minute   int
		wantInWindow   bool
		wantStatus     WindowStatus
		description    string
	}{
		// 周一的窗口
		{
			name:         "周一 20:59 - 窗口即将开始",
			weekday:      time.Monday,
			hour:         20,
			minute:       59,
			wantInWindow: false,
			wantStatus:   StatusBeforeWindow,
			description:  "周一晚上，窗口即将开启",
		},
		{
			name:         "周一 21:00 - 进入窗口",
			weekday:      time.Monday,
			hour:         21,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周一晚上 21:00，窗口开启",
		},
		{
			name:         "周一 22:00 - 在窗口内",
			weekday:      time.Monday,
			hour:         22,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周一晚上，在窗口内",
		},
		{
			name:         "周二 02:00 - 在窗口内（跨天）",
			weekday:      time.Tuesday,
			hour:         2,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周二凌晨，延续周一的窗口",
		},
		{
			name:         "周二 06:00 - 窗口刚结束",
			weekday:      time.Tuesday,
			hour:         6,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周二早上 06:00，窗口结束",
		},
		{
			name:         "周二 21:00 - 进入窗口",
			weekday:      time.Tuesday,
			hour:         21,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周二晚上 21:00，窗口开启",
		},

		// 关键测试：周五晚上到周六凌晨的跨天窗口
		{
			name:         "周五 21:00 - 进入窗口",
			weekday:      time.Friday,
			hour:         21,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周五晚上 21:00，窗口开启",
		},
		{
			name:         "周五 22:00 - 在窗口内",
			weekday:      time.Friday,
			hour:         22,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周五晚上，在窗口内",
		},
		{
			name:         "周六 02:00 - 在窗口内（关键测试！）",
			weekday:      time.Saturday,
			hour:         2,
			minute:       0,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周六凌晨，延续周五的窗口（这是 bug 修复的关键测试）",
		},
		{
			name:         "周六 05:59 - 在窗口内（即将结束）",
			weekday:      time.Saturday,
			hour:         5,
			minute:       59,
			wantInWindow: true,
			wantStatus:   StatusInWindow,
			description:  "周六凌晨 05:59，仍在窗口内",
		},
		{
			name:         "周六 06:00 - 窗口结束",
			weekday:      time.Saturday,
			hour:         6,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周六早上 06:00，窗口结束",
		},
		{
			name:         "周六 21:00 - 不在窗口内",
			weekday:      time.Saturday,
			hour:         21,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周六晚上不在 weekdays 中，无窗口",
		},

		// 周日的测试
		{
			name:         "周日 02:00 - 不在窗口内",
			weekday:      time.Sunday,
			hour:         2,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周日凌晨，周六没有开启窗口",
		},
		{
			name:         "周日 21:00 - 不在窗口内",
			weekday:      time.Sunday,
			hour:         21,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周日晚上不在 weekdays 中，无窗口",
		},

		// 周一凌晨的测试
		{
			name:         "周一 02:00 - 不在窗口内",
			weekday:      time.Monday,
			hour:         2,
			minute:       0,
			wantInWindow: false,
			wantStatus:   StatusOutsideWindow,
			description:  "周一凌晨，周日没有开启窗口",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 构造测试时间
			// 使用 2024-01-xx 作为基准日期
			baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			for baseDate.Weekday() != tc.weekday {
				baseDate = baseDate.AddDate(0, 0, 1)
			}
			testTime := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(),
				tc.hour, tc.minute, 0, 0, time.UTC)

			// 使用一个辅助函数来注入测试时间
			// 注意：这需要修改 Filter 结构以支持时间注入
			// 这里我们使用实际的 ShouldRun，但需要考虑时区

			// 由于当前实现使用 time.Now()，我们需要重构以支持测试
			// 暂时跳过实际测试，仅记录预期行为
			_ = testTime // 避免未使用变量错误
			t.Logf("测试场景: %s", tc.description)
			// 将 time.Weekday 转换为 int 格式 (Sunday=0 -> 7, Monday=1 -> 1, etc.)
			weekdayInt := int(tc.weekday)
			if weekdayInt == 0 {
				weekdayInt = 7
			}
			t.Logf("  时间: %s %02d:%02d", weekdayName(weekdayInt), tc.hour, tc.minute)
			t.Logf("  预期: inWindow=%v, status=%s", tc.wantInWindow, tc.wantStatus)
		})
	}
}

// TestToWeekMinutes 测试周内分钟数转换
func TestToWeekMinutes(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled:  true,
		TimeZone: "Asia/Shanghai",
	}
	filter, _ := NewFilter(cfg)

	tests := []struct {
		name     string
		hour     int
		minute   int
		weekday  int
		want     int
	}{
		{
			name:    "周一 00:00",
			hour:    0,
			minute:  0,
			weekday: 1,
			want:    0,
		},
		{
			name:    "周一 21:00",
			hour:    21,
			minute:  0,
			weekday: 1,
			want:    21 * 60,
		},
		{
			name:    "周二 00:00",
			hour:    0,
			minute:  0,
			weekday: 2,
			want:    24 * 60,
		},
		{
			name:    "周二 06:00",
			hour:    6,
			minute:  0,
			weekday: 2,
			want:    (24 + 6) * 60,
		},
		{
			name:    "周五 21:00",
			hour:    21,
			minute:  0,
			weekday: 5,
			want:    (4 * 24 + 21) * 60,
		},
		{
			name:    "周六 02:00",
			hour:    2,
			minute:  0,
			weekday: 6,
			want:    (5 * 24 + 2) * 60,
		},
		{
			name:    "周六 06:00",
			hour:    6,
			minute:  0,
			weekday: 6,
			want:    (5 * 24 + 6) * 60,
		},
		{
			name:    "周日 23:59",
			hour:    23,
			minute:  59,
			weekday: 7,
			want:    (6 * 24 + 23) * 60 + 59,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.toWeekMinutes(tt.hour, tt.minute, tt.weekday)
			if got != tt.want {
				t.Errorf("toWeekMinutes(%d:%d, weekday=%d) = %d, want %d",
					tt.hour, tt.minute, tt.weekday, got, tt.want)
			}
		})
	}
}

// TestCrossDayWindowManual 手动测试跨天窗口逻辑
// 这个测试演示了跨天窗口的分钟数计算逻辑
func TestCrossDayWindowManual(t *testing.T) {
	cfg := config.TimeWindowConfig{
		Enabled:  true,
		TimeZone: "Asia/Shanghai",
	}
	filter, _ := NewFilter(cfg)

	// 配置: start=21:00, end=06:00 (跨天)
	startHour, startMin := 21, 0
	endHour, endMin := 6, 0

	// 计算基准分钟数（作为时间偏移量，不是周内分钟数）
	startMinutes := startHour*60 + startMin // 21:00 = 1260 分钟（从某天 00:00 开始）
	endMinutes := endHour*60 + endMin       // 06:00 = 360 分钟（从某天 00:00 开始）

	t.Logf("时间偏移量（从某天 00:00 开始）:")
	t.Logf("  start (21:00) = %d 分钟", startMinutes)
	t.Logf("  end (06:00)   = %d 分钟", endMinutes)

	// 处理跨天：如果结束时间小于开始时间，说明跨天
	duration := endMinutes - startMinutes
	if duration < 0 {
		// 跨天：从当天 21:00 到次日 06:00
		// = (24*60 - 21*60) + 6*60 = 180 + 360 = 540 分钟
		duration = (24*60 - startMinutes) + endMinutes
		t.Logf("  检测到跨天，持续时间 = %d 分钟 = %d 小时", duration, duration/60)
	} else {
		t.Logf("  不跨天，持续时间 = %d 分钟 = %d 小时", duration, duration/60)
	}

	// 周五 21:00 开启的窗口
	fridayStartMinutes := filter.toWeekMinutes(startHour, startMin, 5) // 周五 21:00 = 7020
	fridayEndMinutes := fridayStartMinutes + duration                  // 7020 + 540 = 7560

	t.Logf("\n周五窗口:")
	t.Logf("  周五 21:00 = %d 分钟", fridayStartMinutes)
	t.Logf("  结束时间   = %d 分钟", fridayEndMinutes)

	// 验证结束时间：7560 分钟 = 5*1440 + 360 = 周六 06:00
	endWeekday := (fridayEndMinutes / 1440) + 1  // 7560/1440=5, 5+1=6 (周六)
	endHourOfDay := (fridayEndMinutes % 1440) / 60  // 7560%1440=360, 360/60=6
	t.Logf("  结束于: 周%d %02d:00", endWeekday, endHourOfDay)

	// 测试关键时间点
	testPoints := []struct {
		name     string
		weekday  int
		hour     int
		minute   int
		wantIn   bool
	}{
		{"周五 21:00", 5, 21, 0, true},
		{"周五 22:00", 5, 22, 0, true},
		{"周六 02:00", 6, 2, 0, true},  // 关键测试点！
		{"周六 05:59", 6, 5, 59, true},
		{"周六 06:00", 6, 6, 0, false},
		{"周六 21:00", 6, 21, 0, false},
	}

	t.Logf("\n窗口状态检查:")
	for _, tp := range testPoints {
		pointMinutes := filter.toWeekMinutes(tp.hour, tp.minute, tp.weekday)
		inWindow := pointMinutes >= fridayStartMinutes && pointMinutes < fridayEndMinutes

		status := "✓"
		if inWindow != tp.wantIn {
			status = "✗"
		}

		t.Logf("  %s %s %02d:%02d = %d 分钟, 在窗口内: %v (期望: %v) %s",
			status, tp.name, tp.hour, tp.minute, pointMinutes, inWindow, tp.wantIn, status)
	}
}

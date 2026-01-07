package main

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

// StreamHandler 流消息处理器（处理价格更新）
type StreamHandler struct {
	symbol     string
	mu         sync.Mutex
	lastPrice  float64
	priceCount int64
	startTime  time.Time
}

// OnPriceUpdate 处理价格更新（实现 ws.PriceHandler 接口）
func (h *StreamHandler) OnPriceUpdate(data ws.PriceData) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 解析价格
	markPrice, err := strconv.ParseFloat(data.Data.MarkPrice, 64)
	if err != nil {
		slog.Error("parse mark price failed", "error", err)
		return
	}

	lastPrice, err := strconv.ParseFloat(data.Data.LastPrice, 64)
	if err != nil {
		slog.Error("parse last price failed", "error", err)
		return
	}

	midPrice, err := strconv.ParseFloat(data.Data.MidPrice, 64)
	if err != nil {
		slog.Error("parse mid price failed", "error", err)
		return
	}

	h.lastPrice = markPrice
	h.priceCount++

	// 格式化时间显示
	ts := time.Now().Format("15:04:05.000")

	// 价格变化指示器
	var changeIndicator string
	if h.priceCount > 1 {
		if markPrice > h.lastPrice {
			changeIndicator = "📈"
		} else if markPrice < h.lastPrice {
			changeIndicator = "📉"
		} else {
			changeIndicator = "➡️"
		}
	} else {
		changeIndicator = "✨"
	}

	// 打印价格更新
	fmt.Printf("[%s] %s 📊 PRICE %-8s | Mark: $%.2f | Last: $%.2f | Mid: $%.2f | Spread: [%s, %s]\n",
		ts,
		changeIndicator,
		data.Data.Symbol,
		markPrice,
		lastPrice,
		midPrice,
		data.Data.Spread[0],
		data.Data.Spread[1])

	// 调试模式下打印更多信息
	if slog.Default().Handler().Enabled(nil, slog.LevelDebug) {
		slog.Debug("price data",
			"seq", data.Seq,
			"index_price", data.Data.IndexPrice,
			"time", data.Data.Time)
	}
}

// printSummary 打印最终摘要
func (h *StreamHandler) printSummary() {
	h.mu.Lock()
	defer h.mu.Unlock()

	duration := time.Since(h.startTime)

	fmt.Printf("\n📊 Symbol: %s\n", h.symbol)
	fmt.Printf("   Last Mark Price: $%.2f\n", h.lastPrice)
	fmt.Printf("   Total Price Updates: %d\n", h.priceCount)

	if h.priceCount > 0 {
		// 计算消息速率
		rate := float64(h.priceCount) / duration.Seconds()
		fmt.Printf("   Duration: %s\n", duration.Round(time.Millisecond))
		fmt.Printf("   Price Update Rate: %.2f msg/sec\n", rate)
	}
}

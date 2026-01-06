package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

// StreamHandler 流消息处理器
type StreamHandler struct {
	symbol      string
	mu          sync.Mutex
	lastPrice   float64
	lastPriceTs int64
	priceCount  int64
	orders      map[string]*ws.OrderUpdateMessage
	orderCount  int
}

// OnPriceUpdate 处理价格更新
func (h *StreamHandler) OnPriceUpdate(msg ws.PriceMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastPrice = msg.Price
	h.lastPriceTs = msg.Timestamp
	h.priceCount++

	// 格式化时间显示
	ts := time.UnixMilli(msg.Timestamp).Format("15:04:05.000")

	// 打印价格更新
	fmt.Printf("[%s] 📊 PRICE %-8s | Price: $%.2f | Updates: %d\n",
		ts, msg.Symbol, msg.Price, h.priceCount)
}

// OnOrderUpdate 处理订单更新
func (h *StreamHandler) OnOrderUpdate(msg ws.OrderUpdateMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 保存订单状态
	h.orders[msg.OrderID] = &msg
	h.orderCount++

	// 格式化时间显示
	ts := time.Now().Format("15:04:05")

	// 订单状态图标
	var icon string
	switch msg.Status {
	case "new", "open":
		icon = "🆕"
	case "filled":
		icon = "✅"
	case "partial_filled":
		icon = "⏳"
	case "canceled":
		icon = "❌"
	case "rejected":
		icon = "⛔"
	default:
		icon = "📋"
	}

	// 打印订单更新
	fmt.Printf("[%s] %s ORDER %-12s | Status: %-15s | Filled: %.4f\n",
		ts, icon, msg.OrderID, msg.Status, msg.FilledQty)

	if msg.Order != nil {
		fmt.Printf("     └─ Symbol: %s | Side: %s | Price: %s | Qty: %s\n",
			msg.Order.Symbol, msg.Order.Side, msg.Order.Price, msg.Order.Qty)
	}
}

// OnPositionUpdate 处理仓位更新
func (h *StreamHandler) OnPositionUpdate(msg ws.PositionUpdateMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ts := time.Now().Format("15:04:05")

	var sideIcon string
	if msg.Side == "long" {
		sideIcon = "📈"
	} else {
		sideIcon = "📉"
	}

	fmt.Printf("[%s] %s POSITION %-8s | Size: %.4f | Unrealized PnL: $%.2f\n",
		ts, sideIcon, msg.Symbol, msg.Size, msg.UnrealizedPnL)
}

// printSummary 打印最终摘要
func (h *StreamHandler) printSummary() {
	h.mu.Lock()
	defer h.mu.Unlock()

	fmt.Printf("\n📊 Symbol: %s\n", h.symbol)
	fmt.Printf("   Last Price: $%.2f\n", h.lastPrice)
	fmt.Printf("   Price Updates: %d\n", h.priceCount)
	fmt.Printf("\n📋 Total Order Updates: %d\n", h.orderCount)

	if len(h.orders) > 0 {
		fmt.Println("\n   Order Status:")
		for _, order := range h.orders {
			var statusIcon string
			switch order.Status {
			case "new", "open":
				statusIcon = "🆕"
			case "filled":
				statusIcon = "✅"
			case "partial_filled":
				statusIcon = "⏳"
			case "canceled":
				statusIcon = "❌"
			case "rejected":
				statusIcon = "⛔"
			default:
				statusIcon = "📋"
			}
			fmt.Printf("   %s %s: %s (Filled: %.4f)\n",
				statusIcon, order.OrderID, order.Status, order.FilledQty)
		}
	}
}

// OnLoginSuccess 登录成功回调
func (h *StreamHandler) OnLoginSuccess() {
	fmt.Println("✅ WebSocket login successful")
}

// OnError 错误回调
func (h *StreamHandler) OnError(code int, message string) {
	fmt.Printf("⚠️  Error: [%d] %s\n", code, message)
}

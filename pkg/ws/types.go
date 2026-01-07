package ws

import (
	"encoding/json"
)

// SubscribeRequest 订阅请求
type SubscribeRequest struct {
	Subscribe SubscribeParams `json:"subscribe"`
}

// SubscribeParams 订阅参数
type SubscribeParams struct {
	Channel string `json:"channel"`
	Symbol  string `json:"symbol,omitempty"`
}

// AuthRequest 认证请求（用于订阅 order channel）
type AuthRequest struct {
	Auth    AuthParams `json:"auth"`
}

// AuthParams 认证参数
type AuthParams struct {
	Token   string       `json:"token"`
	Streams []StreamSpec `json:"streams"`
}

// StreamSpec 流规格
type StreamSpec struct {
	Channel string `json:"channel"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	Seq     int        `json:"seq"`
	Channel string     `json:"channel"`
	Data    AuthDetail `json:"data"`
}

// AuthDetail 认证详情
type AuthDetail struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// PriceData 价格数据（完整结构）
type PriceData struct {
	Seq       int64       `json:"seq"`
	Channel   string      `json:"channel"`
	Symbol    string      `json:"symbol"`
	Data      PriceDetail `json:"data"`
}

// PriceDetail 价格详情
type PriceDetail struct {
	Base       string   `json:"base"`
	IndexPrice string   `json:"index_price"`
	LastPrice  string   `json:"last_price"`
	MarkPrice  string   `json:"mark_price"`
	MidPrice   string   `json:"mid_price"`
	Quote      string   `json:"quote"`
	Spread     []string `json:"spread"` // [bid, ask]
	Symbol     string   `json:"symbol"`
	Time       string   `json:"time"`
}

// OrderData 订单数据
type OrderData struct {
	Seq     int         `json:"seq"`
	Channel string      `json:"channel"`
	Data    OrderDetail `json:"data"`
}

// OrderDetail 订单详情
type OrderDetail struct {
	ID            int    `json:"id"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	OrderType     string `json:"order_type"`
	Price         string `json:"price"`
	Qty           string `json:"qty"`
	FillQty       string `json:"fill_qty"`
	FillAvgPrice  string `json:"fill_avg_price"`
	Status        string `json:"status"`
	TimeInForce   string `json:"time_in_force"`
	ReduceOnly    bool   `json:"reduce_only"`
	Leverage      string `json:"leverage"`
	Margin        string `json:"margin"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	ClOrdID       string `json:"cl_ord_id"`
	PositionID    int    `json:"position_id"`
	AvailLocked   string `json:"avail_locked"`
	User          string `json:"user,omitempty"`
}

// ChannelMessage 频道推送消息（通用）
type ChannelMessage struct {
	Seq     int             `json:"seq"`
	Channel string          `json:"channel"`
	Symbol  string          `json:"symbol,omitempty"`
	Data    json.RawMessage `json:"data"`
}


package ws

import (
	"encoding/json"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// SubscribeRequest 订阅请求
type SubscribeRequest struct {
	Subscribe SubscribeParams `json:"subscribe"`
}

// UnsubscribeRequest 取消订阅请求
type UnsubscribeRequest struct {
	Unsubscribe SubscribeParams `json:"unsubscribe"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Login LoginParams `json:"login"`
}

// SubscribeParams 订阅参数
type SubscribeParams struct {
	Channel string `json:"channel"`
	Symbol  string `json:"symbol,omitempty"`
}

// LoginParams 登录参数
type LoginParams struct {
	Token string `json:"token"`
}

// ChannelMessage 频道推送消息
type ChannelMessage struct {
	Seq     int             `json:"seq"`
	Channel string          `json:"channel"`
	Symbol  string          `json:"symbol,omitempty"`
	Data    json.RawMessage `json:"data"`
}

// PriceData 价格数据
type PriceData struct {
	Base        string  `json:"base"`
	IndexPrice  string  `json:"index_price"`
	LastPrice   string  `json:"last_price"`
	MarkPrice   string  `json:"mark_price"`
	MidPrice    string  `json:"mid_price"`
	Quote       string  `json:"quote"`
	Spread      [2]string `json:"spread"`
	Symbol      string  `json:"symbol"`
	Time        string  `json:"time"`
}

// PriceMessage 价格推送消息
type PriceMessage struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
}

// OrderUpdateMessage 订单更新消息
type OrderUpdateMessage struct {
	OrderID    string          `json:"order_id"`
	Symbol     string          `json:"symbol"`
	Status     string          `json:"status"`
	FilledQty  float64         `json:"filled_qty"`
	Order      *client.Order   `json:"order"`
}

// PositionUpdateMessage 仓位更新消息
type PositionUpdateMessage struct {
	Symbol        string  `json:"symbol"`
	Size          float64 `json:"size"`
	Side          string  `json:"side"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
}

// BalanceUpdateMessage 余额更新消息
type BalanceUpdateMessage struct {
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	Available float64 `json:"available"`
}

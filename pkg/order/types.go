package order

import (
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// OrderPair 双边订单
type OrderPair struct {
	Bid *client.Order
	Ask *client.Order
}

// OrderState 订单状态
type OrderState int

const (
	OrderStateIdle OrderState = iota
	OrderStatePlacing
	OrderStateActive
	OrderStateCanceling
)

// TrackedOrder 被追踪的订单
type TrackedOrder struct {
	Order      *client.Order
	State      OrderState
	UpdateTime time.Time
}

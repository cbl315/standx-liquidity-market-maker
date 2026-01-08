package client

// OrderSide 订单方向
type OrderSide string

const (
	OrderBid OrderSide = "buy"
	OrderAsk OrderSide = "sell"
)

// OrderType 订单类型
type OrderType string

const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

// TimeInForce 订单时效
type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "gtc" // Good Till Cancel
	TimeInForceIOC TimeInForce = "ioc" // Immediate Or Cancel
	TimeInForceFOK TimeInForce = "fok" // Fill Or Kill
)

// NewOrderRequest 下单请求
type NewOrderRequest struct {
	Symbol      string      `json:"symbol"`
	Side        OrderSide   `json:"side"`
	OrderType   OrderType   `json:"order_type"`
	Qty         string      `json:"qty"`
	Price       string      `json:"price,omitempty"`
	SlPrice     string      `json:"sl_price,omitempty"` // 止损价格
	TpPrice     string      `json:"tp_price,omitempty"` // 止盈价格
	TimeInForce TimeInForce `json:"time_in_force"`
	ReduceOnly  bool        `json:"reduce_only"`
	ClOrdID     string      `json:"cl_ord_id,omitempty"` // Client order ID
}

// Order 订单 (根据 API 文档更新)
type Order struct {
	ID            int       `json:"id"`
	Symbol        string    `json:"symbol"`
	Side          OrderSide `json:"side"`
	OrderType     OrderType `json:"order_type"`
	Price         string    `json:"price"`
	Qty           string    `json:"qty"`
	FillQty       string    `json:"fill_qty"`
	FillAvgPrice  string    `json:"fill_avg_price"`
	Status        string    `json:"status"`
	TimeInForce   TimeInForce `json:"time_in_force"`
	ReduceOnly    bool      `json:"reduce_only"`
	Leverage      string    `json:"leverage"`
	Margin        string    `json:"margin"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
	ClOrdID       string    `json:"cl_ord_id"`
	PositionID    int       `json:"position_id"`
	AvailLocked   string    `json:"avail_locked"`
}

// OrderResponse 下单响应
type OrderResponse struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	Data     struct {
		OrderID string `json:"order_id"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// OrderID returns the order ID from response
func (r *OrderResponse) OrderID() string {
	return r.Data.OrderID
}

// OpenOrdersResponse 开放订单响应
type OpenOrdersResponse struct {
	PageSize int     `json:"page_size"`
	Result   []Order `json:"result"`
	Total    int     `json:"total"`
}

// Position 仓位 (根据 API 文档更新)
type Position struct {
	ID               int     `json:"id"`
	Symbol           string  `json:"symbol"`
	Qty              string  `json:"qty"`
	EntryPrice       string  `json:"entry_price"`
	MarkPrice        string  `json:"mark_price"`
	PositionValue    string  `json:"position_value"`
	HoldingMargin    string  `json:"holding_margin"`
	Leverage         string  `json:"leverage"`
	MarginMode       string  `json:"margin_mode"`
	LiqPrice         string  `json:"liq_price"`
	BankruptcyPrice  string  `json:"bankruptcy_price"`
	UPnL             string  `json:"upnl"`
	RealizedPnL      string  `json:"realized_pnl"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// SymbolPriceInfo Symbol 价格信息 (根据 API 文档更新)
type SymbolPriceInfo struct {
	Symbol      string `json:"symbol"`
	Base        string `json:"base"`
	Quote       string `json:"quote"`
	MarkPrice   string `json:"mark_price"`
	IndexPrice  string `json:"index_price"`
	LastPrice   string `json:"last_price"`
	MidPrice    string `json:"mid_price"`
	SpreadBid   string `json:"spread_bid"`
	SpreadAsk   string `json:"spread_ask"`
	Time        string `json:"time"`
}

// Trade 成交记录
type Trade struct {
	ID        int    `json:"id"`
	OrderID   int    `json:"order_id"`
	User      string `json:"user"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Price     string `json:"price"`
	Qty       string `json:"qty"`
	Value     string `json:"value"`
	FeeAsset  string `json:"fee_asset"`
	FeeQty    string `json:"fee_qty"`
	PNL       string `json:"pnl"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TradesResponse 成交记录响应
type TradesResponse struct {
	PageSize int     `json:"page_size"`
	Result   []Trade `json:"result"`
	Total    int     `json:"total"`
}

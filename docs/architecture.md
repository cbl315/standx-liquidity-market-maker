# StandX 做市机器人架构设计

## 项目概述

自动挂单机器人，用于参与 StandX Market Maker Uptime Program，通过持续提供流动性获取积分奖励。

### 目标

- 自动维持双边挂单（bid + ask）
- 订单价格保持在中间价 10 bps 以内
- 每小时保持 70%+ 在线时间
- 订单规模接近 2 BTC 上限
- **风险控制**: 吃单后立即平仓，不累积仓位

## 技术选型

| 技术栈 | 选择 | 理由 |
|--------|------|------|
| 语言 | Go 1.21+ | 高并发、高性能、部署简单 |
| 加密 | crypto/ed25519 | 标准库，无需外部依赖 |
| Base58 | github.com/mr-tron/base58 | requestId 编码 |
| 以太坊 | github.com/ethereum/go-ethereum | 钱包签名 |
| HTTP | net/http | 标准库 |
| WebSocket | gorilla/websocket | 成熟的 WebSocket 库 |
| 配置 | github.com/spf13/viper | 灵活配置管理 |
| 日志 | slog (Go 1.21+) | 结构化日志 |

## 模块设计

### 1. 认证模块 (pkg/auth)

**职责**: 处理 StandX API 认证

**核心流程**:
```
1. 生成 ed25519 密钥对
2. requestId = base58(publicKey)
3. prepare-signin 获取 signedData
4. 钱包签名 message
5. login 获取 access token
```

**接口定义**:
```go
type Auth struct {
    ed25519PrivateKey ed25519.PrivateKey
    ed25519PublicKey  ed25519.PublicKey
    requestId         string
    baseURL           string
}

func (a *Auth) Authenticate(chain, address string, signFn SignFunc) (*LoginResponse, error)
func (a *Auth) SignRequest(payload []byte, reqID string, timestamp int64) (*RequestSignature, error)
```

**关键类型**:
```go
type Chain string // "bsc" | "solana"

type SignedData struct {
    Domain    string
    URI       string
    Statement string
    Version   string
    ChainID   int
    Nonce     string
    Address   string
    RequestID string
    Message   string
    Exp       int
    Iat       int
}

type LoginResponse struct {
    Token      string
    Address    string
    Alias      string
    Chain      string
    PerpsAlpha bool
}
```

### 2. API 客户端 (pkg/client)

**职责**: 封装 StandX API 调用，自动处理请求签名

**核心功能**:
- 自动添加认证头
- 请求签名 (Body Signature Flow)
- 错误处理和重试
- Token 刷新

### 2.5 WebSocket 客户端 (pkg/ws)

**职责**: 实时接收市场价格数据和订单状态更新

**核心功能**:
- 订阅价格推送
- Ping/Pong 心跳维持（服务器每 10s Ping，5 分钟无响应断开）
- 自动重连

> **注意**: 订单 channel 订阅已移除，现在通过 HTTP API 查询订单获取 `cl_ord_id`

**接口定义**:
```go
type WSClient struct {
    conn        *websocket.Conn
    url         string
    priceHandler PriceHandler
    orderHandler OrderHandler
    reconnectCh chan struct{}
    done        chan struct{}
}

type PriceHandler interface {
    OnPriceUpdate(PriceData)
}

func (ws *WSClient) Connect() error
func (ws *WSClient) SubscribePrice(symbol string) error
func (ws *WSClient) Close() error
```

> **注意**: `OrderHandler` 和 `SubscribeOrder()` 已移除，订单信息通过 HTTP API 查询

**价格数据结构**:
```go
type PriceData struct {
    Seq        int64     `json:"seq"`
    Channel    string    `json:"channel"`
    Symbol     string    `json:"symbol"`
    Data       PriceDetail `json:"data"`
}

type PriceDetail struct {
    Base       string  `json:"base"`
    IndexPrice string  `json:"index_price"`
    LastPrice  string  `json:"last_price"`
    MarkPrice  string  `json:"mark_price"`
    MidPrice   string  `json:"mid_price"`
    Quote      string  `json:"quote"`
    Spread     []string `json:"spread"`  // [bid, ask]
    Symbol     string  `json:"symbol"`
    Time       string  `json:"time"`
}
```

**订单数据结构**:
```go
type OrderData struct {
    Seq        int64     `json:"seq"`
    Channel    string    `json:"channel"`
    Data       OrderDetail `json:"data"`
}

type OrderDetail struct {
    ID            int       `json:"id"`
    Symbol        string    `json:"symbol"`
    Side          string    `json:"side"`
    OrderType     string    `json:"order_type"`
    Price         string    `json:"price"`
    Qty           string    `json:"qty"`
    FillQty       string    `json:"fill_qty"`
    FillAvgPrice  string    `json:"fill_avg_price"`
    Status        string    `json:"status"`
    TimeInForce   string    `json:"time_in_force"`
    ReduceOnly    bool      `json:"reduce_only"`
    Leverage      string    `json:"leverage"`
    Margin        string    `json:"margin"`
    CreatedAt     string    `json:"created_at"`
    UpdatedAt     string    `json:"updated_at"`
    ClOrdID       string    `json:"cl_ord_id"`       // 平台生成的 client order ID
    PositionID    int       `json:"position_id"`
    AvailLocked   string    `json:"avail_locked"`
}
```

**订阅请求格式**:
```json
// 订阅价格（无需认证）
{
  "subscribe": {
    "channel": "price",
    "symbol": "BTC-USD"
  }
}

// 订阅订单（需要先认证）
// Step 1: 使用 JWT token 认证
{
  "auth": {
    "token": "<your_jwt_token>",
    "streams": [{ "channel": "order" }]
  }
}

// Step 2: 认证成功后订阅订单
{
  "subscribe": {
    "channel": "order"
  }
}
```

**认证响应**:
```json
{
  "seq": 1,
  "channel": "auth",
  "data": { "code": 200, "msg": "success" }
}
```

**订单推送响应示例**:
```json
{
  "seq": 35,
  "channel": "order",
  "data": {
    "id": 2547027,
    "symbol": "BTC-USD",
    "side": "buy",
    "order_type": "market",
    "price": "121245.20",
    "qty": "1.000",
    "fill_qty": "1.000",
    "fill_avg_price": "121245.21",
    "status": "filled",
    "time_in_force": "ioc",
    "reduce_only": false,
    "leverage": "15",
    "margin": "8083.013333334",
    "cl_ord_id": "01K2C9H93Y42RW8KD6RSVWVDVV",
    "position_id": 15,
    "avail_locked": "0",
    "created_at": "2025-08-11T10:06:37.182464902Z",
    "updated_at": "2025-08-11T10:06:37.182465022Z",
    "user": "bsc_0x..."
  }
}
```

**WebSocket 端点**:
```
wss://perps.standx.com/ws-stream/v1
```

**Client Order ID 管理策略**:
1. 创建订单时不传入 `cl_ord_id`，由平台自动生成
2. 取消订单时，通过 HTTP API 查询当前 open 订单获取 `cl_ord_id`
3. 使用查询到的 `cl_ord_id` 调用取消订单接口

> **优点**: 消除了 WebSocket 异步接收 `cl_ord_id` 导致的竞态条件问题

### 3. 订单管理器 (pkg/order)

**职责**: 管理订单生命周期

**核心功能**:
- 创建双边挂单（限价单 + 止损/止盈）
- 自动刷新订单
- 取消订单（通过 HTTP API 查询获取 cl_ord_id）

**接口定义**:
```go
type Manager struct {
    client    *client.Client
    bidOrder  *TrackedOrder
    askOrder  *TrackedOrder
    mutex     sync.RWMutex
}

type TrackedOrder struct {
    Order      *client.Order
    State      OrderState
    UpdateTime time.Time
}

func (m *Manager) PlaceBidAsk(bidPrice, askPrice, size float64) error
func (m *Manager) UpdateOrders(newBidPrice, newAskPrice float64) error
func (m *Manager) CancelAll() error
func (m *Manager) GetActiveOrders() (*OrderPair, error)
```

**Client Order ID 管理流程**:
```go
// 1. 创建订单时不传入 cl_ord_id，由平台自动生成
req := &client.NewOrderRequest{
    Symbol:      symbol,
    Side:        client.OrderBid,
    OrderType:   client.OrderTypeLimit,
    Qty:         client.FormatQty(qty),
    Price:       client.FormatPrice(price),
    SlPrice:     client.FormatPrice(slPrice),
    TpPrice:     client.FormatPrice(tpPrice),
    TimeInForce: client.TimeInForceGTC,
    ReduceOnly:  false,
    // ClOrdID 不传，由平台自动生成
}

// 2. 订单创建成功，无需保存 cl_ord_id（取消时通过查询获取）

// 3. 取消订单时，先查询当前 open 订单
func (m *Manager) cancelBidOrder() error {
    // 查询当前 open 订单
    orders, err := m.client.GetOpenOrdersByStatus(m.symbol, "new")
    if err != nil {
        return err
    }

    // 找到 buy side 的订单
    var clOrdID string
    for _, ord := range orders {
        if ord.Side == "buy" {
            clOrdID = ord.ClOrdID
            break
        }
    }

    // 使用查询到的 cl_ord_id 取消订单
    return m.client.CancelOrder(clOrdID)
}
```

**订单创建参数**:
```go
type CreateOrderRequest struct {
    Symbol     string  `json:"symbol"`     // "BTC-USD"
    Side       string  `json:"side"`       // "bid" | "ask"
    OrderType  string  `json:"order_type"` // "limit"
    Qty        string  `json:"qty"`        // 订单数量 (BTC)
    Price      string  `json:"price"`      // 限价
    SlPrice    string  `json:"sl_price"`   // 止损价格 (开单价 ±2bp)
    TpPrice    string  `json:"tp_price"`   // 止盈价格 (开单价 ±2bp)
    ClOrdID    string  `json:"cl_ord_id,omitempty"` // 不传，由平台生成
}
```

**止损/止盈价格计算**:
```
N bp = 配置中的 sl_tp_bps (默认 2bp = 0.02%)

Bid 单（买入）:
- 开仓价格 = bidPrice
- 止损价格 = bidPrice × (1 - N/10000)  // 低于开仓 N bp
- 止盈价格 = bidPrice × (1 + N/10000)  // 高于开仓 N bp

Ask 单（卖出）:
- 开仓价格 = askPrice
- 止损价格 = askPrice × (1 + N/10000)  // 高于开仓 N bp
- 止盈价格 = askPrice × (1 - N/10000)  // 低于开仓 N bp

无论价格涨跌，都在 N bp 范围内自动平仓
```

### 4. 做市策略 (pkg/strategy)

**职责**: 实现做市逻辑，决定挂单价格和规模

**核心算法**:
```
1. WebSocket 接收价格更新
2. 计算挂单价格:
   - bid = markPrice × (1 - spread/2)
   - ask = markPrice × (1 + spread/2)
3. 确保在 10 bps 以内
4. 订单规模 = 2 BTC (可配置)
```

**接口定义**:
```go
type MarketMaker struct {
    orderMgr  *order.Manager
    pricer    *Pricer
    config    *Config
}

func (mm *MarketMaker) Run(ctx context.Context) error
func (mm *MarketMaker) calculatePrices() (bid, ask float64, err error)
```

### 5. 风险管理 (pkg/risk)

**职责**: 资金检查，确保有足够余额开单

**风险场景分析**:
```
价格快速上涨: ask 单被成交 → 卖出 BTC → 空头仓位
价格快速下跌: bid 单被成交 → 买入 BTC → 多头仓位
```

**采用策略: SL/TP 自动平仓**

核心原则：
- 开单时同时设置止损（SL）和止盈（TP）
- SL/TP 价格在开单价 ±N bp 范围内（可配置，默认 1-2bp）
- 吃单后通过 SL/TP 自动平仓，无需代码主动处理
- 仅在资金不足时暂停开单，等待 SL/TP 触发后恢复

**优势**:
- ✅ 代码逻辑大幅简化
- ✅ 无需监听订单成交事件
- ✅ 无需手动平仓操作
- ✅ 风险可控（最多损失 N bp）

**接口定义**:
```go
type RiskManager struct {
    client           *client.Client
    minBalanceRatio  float64  // 最低余额比例 (0.2 = 保留20%)
}

// 检查是否有足够余额开单
func (rm *RiskManager) HasSufficientBalance(orderSize float64) (bool, error)

// 获取当前账户余额
func (rm *RiskManager) GetBalance() (*Balance, error)
```

**SL/TP 价格自动平仓机制**:
```
Bid 单（买入）成交后 → 持有多头 BTC
  → TP 触发: 价格达到 bidPrice × (1 + N bp) → 自动卖出平仓
  → SL 触发: 价格达到 bidPrice × (1 - N bp) → 自动卖出平仓

Ask 单（卖出）成交后 → 持有空头
  → TP 触发: 价格达到 askPrice × (1 - N bp) → 自动买入平仓
  → SL 触发: 价格达到 askPrice × (1 + N bp) → 自动买入平仓

无论价格涨跌，都在 N bp 范围内自动平仓
```

**资金检查流程**:
```go
func (rm *RiskManager) HasSufficientBalance(orderSize float64) (bool, error) {
    balance, err := rm.GetBalance()
    if err != nil {
        return false, err
    }

    // 计算所需保证金（简化计算）
    required := orderSize * balance.MarkPrice * rm.minBalanceRatio

    available, _ := strconv.ParseFloat(balance.Available, 64)
    return available >= required, nil
}
```

### 6. 在线监控 (pkg/monitor)

**职责**: 追踪在线时间和积分指标

**接口定义**:
```go
type UptimeMonitor struct {
    hourStart    time.Time
    activeMinutes int
    lastUpdate   time.Time
}

func (m *UptimeMonitor) RecordActive() error
func (m *UptimeMonitor) GetUptimePercentage() float64
func (m *UptimeMonitor) ShouldBeInBoostedTier() bool
```

## 配置设计

### config.yaml

```yaml
# 链配置
chain: bsc  # bsc | solana

# 钱包配置 (通过环境变量)
# WALLET_PRIVATE_KEY

# StandX API
api:
  base_url: https://api.standx.com
  perps_url: https://perps.standx.com
  timeout: 30s

# 做市策略配置
strategy:
  symbol: BTC-USD
  order_qty: 2.0         # BTC
  spread_bps: 8          # 挂单价差 (10 bps 以内)

# 风险管理配置（SL/TP 自动平仓）
risk:
  enabled: true               # 是否启用风险管理
  sl_tp_bps: 2                # SL/TP 价格浮动范围 (1-2bp)
  min_balance_ratio: 0.2      # 最低余额比例 (保留20%用于保证金)
  check_interval: 1s          # 余额检查间隔

# WebSocket 配置
websocket:
  url: wss://perps.standx.com/ws-stream/v1
  ping_interval: 10s     # 服务器 Ping 间隔
  pong_timeout: 5m       # Pong 超时断开时间
  reconnect_delay: 5s    # 重连延迟

# 监控配置
monitor:
  uptime_threshold: 0.70  # 70% 在线时间目标
  log_interval: 5m        # 状态日志间隔

# 日志配置
log:
  level: info  # debug | info | warn | error
  format: json # json | text
```

## 环境变量

### 必需环境变量

| 变量名 | 说明 | 示例 |
|--------|------|------|
| `WALLET_PRIVATE_KEY` | 钱包私钥（64位十六进制） | `0x1234...abcd` |

> ⚠️ **安全提示**: 私钥不应直接写入配置文件或代码中，请使用环境变量或密钥管理工具。

### 可选环境变量

目前所有可选配置都通过 `configs/config.yaml` 文件管理。如需覆盖配置，可以在运行时设置：

```bash
# 示例：通过环境变量覆盖配置（需要修改代码支持）
CHAIN=solana ./bin/market-maker
```

### 配置文件与代码对应关系

| 配置项 | 配置路径 | 代码中使用 | 说明 |
|--------|----------|-----------|------|
| 链配置 | `chain` | `cfg.Chain` | bsc 或 solana |
| API 基础 URL | `api.base_url` | `cfg.API.BaseURL` | 认证服务地址 |
| API Perps URL | `api.perps_url` | `cfg.API.PerpsURL` | Perps API 地址 |
| 交易对 | `strategy.symbol` | `cfg.Strategy.Symbol` | 如 BTC-USD |
| 订单数量 | `strategy.order_qty` | `cfg.Strategy.OrderQty` | 订单数量（BTC） |
| 价差基点 | `strategy.spread_bps` | `cfg.Strategy.SpreadBPS` | 挂单价差 |
| 风险管理开关 | `risk.enabled` | `cfg.Risk.Enabled` | 是否启用余额检查 |
| SL/TP 基点 | `risk.sl_tp_bps` | `cfg.Risk.SlTpBPS` | 止损止盈浮动范围 |
| 最低余额比例 | `risk.min_balance_ratio` | `cfg.Risk.MinBalanceRatio` | 保留保证金比例 |
| WebSocket URL | `websocket.url` | `cfg.WS.URL` | WebSocket 地址 |
| 重连延迟 | `websocket.reconnect_delay` | `cfg.WS.ReconnectDelay` | 断线重连延迟 |
| 日志级别 | `log.level` | `cfg.Log.Level` | 日志详细程度 |
| 日志格式 | `log.format` | `cfg.Log.Format` | json 或 text |

### 启动命令示例

```bash
# 1. 设置私钥环境变量
export WALLET_PRIVATE_KEY=0x...

# 2. 编辑配置文件
vim configs/config.yaml

# 3. 启动做市机器人
./bin/market-maker

# 或使用 Docker
docker run -e WALLET_PRIVATE_KEY=0x... -v $(pwd)/configs:/app/configs standx-mm
```

### 最小配置示例

```yaml
# configs/config.yaml
chain: bsc
strategy:
  symbol: BTC-USD
  order_qty: 0.5       # 小单测试
  spread_bps: 8
risk:
  enabled: true
  sl_tp_bps: 2
  min_balance_ratio: 0.2
websocket:
  url: wss://perps.standx.com/ws-stream/v1
  reconnect_delay: 5s
log:
  level: debug         # 调试模式
  format: text
```

## 配置读取流程

```mermaid
flowchart TD
    Start([启动 market-maker]) --> LoadCfg[加载 configs/config.yaml]
    LoadCfg --> ReadEnv{读取环境变量}

    ReadEnv --> GetKey[WALLET_PRIVATE_KEY]
    GetKey -->|缺失| Error1[❌ 错误: 缺少私钥]
    GetKey -->|存在| CreateWallet[创建钱包签名器]

    CreateWallet --> CreateAuth[创建认证管理器<br/>cfg.API.BaseURL]
    CreateAuth --> DoAuth[执行认证<br/>cfg.Chain]

    DoAuth --> CreateAPIClient[创建 API 客户端<br/>cfg.API.PerpsURL]
    CreateAPIClient --> CreateWS[创建 WebSocket 客户端<br/>cfg.WS.URL]

    CreateWS --> CreateOrderMgr[创建订单管理器<br/>symbol, order_qty, sl_tp_bps]
    CreateOrderMgr --> CreateRiskMgr[创建风险管理器<br/>enabled, min_balance_ratio]

    CreateRiskMgr --> CreateStrategy[创建做市策略<br/>spread_bps]
    CreateStrategy --> Run[启动做市循环]

    Error1 --> Stop([停止])

    style LoadCfg fill:#e3f2fd
    style ReadEnv fill:#fff3e0
    style CreateStrategy fill:#f3e5f5
    style Error1 fill:#ffebee
```

## 配置数据结构

### Config 结构体字段映射

```go
type Config struct {
    Chain    string      // chain: bsc
    API      APIConfig   // api.*
    Strategy Strategy    // strategy.*
    Risk     RiskConfig  // risk.*
    WS       WSConfig    // websocket.*
    Monitor  Monitor     // monitor.*
    Log      LogConfig   // log.*
}
```

### 配置读取优先级

1. **配置文件** (`configs/config.yaml`) - 默认配置
2. **环境变量** - 可覆盖（需要代码支持）
3. **代码默认值** - 兜底逻辑

### 关键配置验证

程序启动时会验证以下关键配置：

| 配置项 | 验证条件 | 错误处理 |
|--------|----------|----------|
| `WALLET_PRIVATE_KEY` | 必须存在且有效 | 程序退出 |
| `chain` | bsc 或 solana | 使用默认值 |
| `strategy.symbol` | 非空 | 程序退出 |
| `strategy.order_qty` | > 0 | 程序退出 |
| `strategy.spread_bps` | 1-10 | 警告日志 |
| `websocket.url` | 有效 URL | 使用默认值 |

## 数据流

```mermaid
flowchart TD
    subgraph WS["WebSocket 连接"]
        WSStream[WebSocket Stream<br/>wss://perps.standx.com/ws-stream/v1]
    end

    subgraph Main["主流程"]
        Start([启动]) --> Auth[认证获取 Token]
        Auth --> WSConn[连接 WebSocket]
        WSConn --> SubPrice[订阅价格: BTC-USD]

        SubPrice --> PriceWS{{接收价格推送}}
        PriceWS -->|价格变动| CalcPrices[计算新价格<br/>within 10 bps]
        CalcPrices --> CheckBalance{检查余额<br/>是否足够?}
        CheckBalance -->|否| WaitBalance[等待 SL/TP<br/>释放保证金]
        CheckBalance -->|是| NeedUpdate{需要更新订单?}
        NeedUpdate -->|是| QueryOrders[查询 open 订单<br/>获取 cl_ord_id]
        QueryOrders --> CancelOrders[取消旧订单]
        CancelOrders --> CreateOrder[创建新订单]
        CreateOrder --> UpdateOrders[更新订单]
        NeedUpdate -->|否| RecordUptime
        UpdateOrders --> RecordUptime[记录在线时间]
        WaitBalance --> RecordUptime

        RecordUptime --> LogStatus[记录状态日志]
        LogStatus --> PriceWS
    end

    subgraph Reconnect["重连机制"]
        WSConn -->|连接断开| Reconnect[等待 5s]
        Reconnect --> WSConn
        WSStream -.->|Ping 每 10s| WSConn
    end

    style WS fill:#e1f5fe
    style Main fill:#f3e5f5
    style Reconnect fill:#fff3e0
    style WaitBalance fill:#fff9c4
    style SaveClOrdID fill:#c8e6c9
    style CancelOrder fill:#ffcc80
```

### SL/TP 自动平仓机制

开单时设置的 SL/TP 由交易所自动处理，代码无需干预：

```
开单: bidPrice=50000, qty=2, sl=49999, tp=50001 (±1bp)

场景1 - 价格上涨:
  → TP 触发 @ 50001 → 自动平多头 → 释放保证金 → 恢复做市

场景2 - 价格下跌:
  → SL 触发 @ 49999 → 自动平多头 → 释放保证金 → 恢复做市

无论涨跌，都在 ±1bp 范围内自动平仓，无需代码处理
```

### WebSocket 消息处理流程

```mermaid
sequenceDiagram
    participant C as Client
    participant WS as WebSocket (wss://perps.standx.com/ws-stream/v1)
    participant S as Strategy
    participant O as OrderManager
    participant API as HTTP API

    C->>WS: 连接
    C->>WS: 订阅价格 {"subscribe": {"channel": "price", "symbol": "BTC-USD"}}

    loop 价格更新
        WS-->>C: PriceUpdate (mark_price, mid_price, spread...)
        C->>S: OnPriceUpdate(price)
        S->>S: 计算新 bid/ask (within 10 bps)
        S->>O: UpdateOrders()

        Note over O,API: 需要更新订单时
        O->>API: GET /api/query_orders?symbol=BTC-USD&status=new
        API-->>O: 返回 open 订单列表 (包含 cl_ord_id)
        O->>API: POST /api/cancel_order (使用查询到的 cl_ord_id)
        O->>API: POST /api/new_order (创建新订单)
    end

    WS-->>C: Ping (每 10s)
    C->>WS: Pong (自动处理)

    Note over C,WS: 连接断开则 5s 后重连
```

**关键变化**:
- ❌ 移除了订单 channel 订阅
- ✅ 使用 HTTP API 查询订单获取 `cl_ord_id`
- ✅ 消除了 WebSocket 异步接收 `cl_ord_id` 的竞态条件

## 错误处理策略

| 错误类型 | 处理方式 |
|----------|----------|
| WebSocket 断开 | 5秒后自动重连 |
| Ping/Pong 超时 | 重连连接 |
| API 超时 | 重试 3 次，指数退避 |
| 认证失败 | 重新认证，记录日志 |
| 订单拒绝 | 检查价格/余额，调整参数 |
| 余额不足 | 暂停开单，等待 SL/TP 触发释放保证金 |
| SL/TP 失败 | 由交易所自动处理，代码无需干预 |

## 风险控制总结

| 控制项 | 限制值 | 说明 |
|--------|--------|------|
| 风险策略 | SL/TP 自动平仓 | 最简单方案 |
| 单笔最大亏损 | N bp | 可配置 (1-2bp) |
| 最大仓位 | N bp | 不累积仓位 |
| 资金检查 | min_balance_ratio | 保留 20% 余额 |

## 安全考虑

1. **私钥管理**
   - 私钥通过环境变量传入
   - 不写入日志
   - 不写入配置文件

2. **API 密钥**
   - Access Token 内存存储
   - 定期刷新机制

3. **请求签名**
   - 每次请求独立签名
   - 时间戳防重放

## 部署建议

### 本地运行
```bash
export WALLET_PRIVATE_KEY=0x...
go run cmd/main.go
```

### Docker 部署
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o mm cmd/main.go

FROM alpine:latest
COPY --from=builder /app/mm /usr/local/bin/
CMD ["mm"]
```

### Systemd 服务
```ini
[Unit]
Description=StandX Market Maker
After=network.target

[Service]
Type=simple
User=standx
Environment="WALLET_PRIVATE_KEY=0x..."
ExecStart=/usr/local/bin/mm
Restart=always

[Install]
WantedBy=multi-user.target
```

## 开发计划

| 阶段 | 任务 | 状态 |
|------|------|------|
| Phase 1 | 认证模块 (pkg/auth) | ✅ 已完成 |
| Phase 2 | WebSocket 客户端 (pkg/ws) | ✅ 已完成 |
| Phase 3 | API 客户端 (pkg/client) | ✅ 已完成 |
| Phase 4 | 订单管理 (pkg/order) | ✅ 已完成 |
| Phase 5 | 风险管理 (pkg/risk) - 立即平仓策略 | ✅ 已完成 |
| Phase 6 | 做市策略 (pkg/strategy) | ✅ 已完成 |
| Phase 7 | 在线监控 (pkg/monitor) | 待开始 |
| Phase 8 | 主程序集成 (cmd/main.go) | ✅ 已完成 |
| Phase 9 | 测试与优化 | 进行中 |

**新增模块**:
- `pkg/timewindow` - 时间窗口控制 (✅ 已完成)

## 已完成优化项

### ✅ 1. 时间窗口控制

**状态**: 已完成

**目标**: 避开高波动时间段（如北京时间工作日晚上 10 点到凌晨 3 点，美股开盘时间）

**实现方案**: 已实现 `pkg/timewindow` 模块

```go
// pkg/timewindow/filter.go
type Filter struct {
    cfg    config.TimeWindowConfig
    loc    *time.Location
}

type WindowStatus string

const (
    StatusInWindow      WindowStatus = "in_window"       // 在窗口期内
    StatusBeforeWindow  WindowStatus = "before_window"   // 窗口期即将开始
    StatusOutsideWindow WindowStatus = "outside_window"  // 在窗口期外
    StatusDisabled      WindowStatus = "disabled"        // 功能未启用
)

func (f *Filter) ShouldRun() (bool, time.Duration, WindowStatus)
func (f *Filter) IsInWindow() bool
func (f *Filter) WaitForWindowEnd() <-chan time.Time
```

**集成位置**:
- `pkg/timewindow/filter.go` - 时间窗口过滤器核心逻辑
- `pkg/config/config.go` - 配置结构体 (`TimeWindowConfig`, `WindowSpec`, `WindowAction`)
- `pkg/strategy/marketmaker.go` - 运行时监控 (`MonitorTimeWindow` 方法)
- `configs/config.yaml` - 配置文件

**功能特性**:
- ✅ 支持跨天时间窗口（如 22:00-03:00）
- ✅ 支持按星期几配置
- ✅ 启动时立即检查当前状态
- ✅ 运行时每 30 秒持续监控
- ✅ 进入窗口期：暂停做市 → 取消订单 → 平仓
- ✅ 离开窗口期：自动恢复做市

**配置示例**:
```yaml
time_window:
  enabled: true
  timezone: "Asia/Shanghai"
  windows:
    - start: "22:00"
      end: "03:00"
      weekdays: [1,2,3,4,5]  # 周一到周五
  action: "shutdown"  # 停止运行
```

---

### ✅ 2. 波动保护机制

**状态**: 已完成

**目标**: 短时间内价格波动超过阈值时暂停做市并平仓

**实现方案**: 已实现 `pkg/volatility` 模块

```go
// pkg/volatility/guard.go
type Guard struct {
    thresholdBPS int                 // 波动阈值（如 50 bps）
    windowSec    int                 // 检测窗口（秒）
    minSnapshots int                 // 最小快照数量
    snapshots    []PriceSnapshot     // 价格历史快照
    enabled      bool                // 是否启用
    isPaused     bool                // 当前是否暂停
    wasPaused    bool                // 上次状态（用于检测变化）
}

type PriceSnapshot struct {
    Timestamp time.Time
    Price     float64
}

func (g *Guard) ShouldPause(currentPrice float64) (bool, int, string)
func (g *Guard) CheckStateChange() (paused, justEntered, justExited bool)
```

**集成位置**:
- `pkg/volatility/guard.go` - 波动保护核心逻辑
- `pkg/strategy/marketmaker.go` - 暂停状态管理 (`PauseState` 枚举)
- `pkg/config/config.go` - 配置结构体 (`VolatilityConfig`)
- `configs/config.yaml` - 配置文件
- `cmd/main.go` - 初始化和启动监控

**功能特性**:
- ✅ 滑动时间窗口计算波动率
- ✅ 可配置波动阈值（bps）和检测窗口时长
- ✅ 最小快照数量保护，避免样本太少误判
- ✅ 波动率计算：`(max - min) / avg * 10000` bps
- ✅ 状态变化检测（进入/退出高波动状态）
- ✅ 进入高波动：暂停 → 取消订单 → 平仓
- ✅ 退出高波动：自动恢复做市
- ✅ 暂停优先级：窗口期 > 高波动
- ✅ 窗口期内跳过波动率监控

**配置示例**:
```yaml
volatility:
  enabled: true              # 是否启用波动保护
  threshold_bps: 50          # 波动阈值（50 bps = 0.5%）
  window_sec: 60             # 检测窗口时长（60秒）
  min_snapshots: 5           # 最小快照数量
```

**暂停状态管理**:
```go
type PauseState int

const (
    PauseStateNone            PauseState = iota  // 未暂停
    PauseStateTimeWindow                      // 时间窗口暂停
    PauseStateHighVolatility                  // 高波动暂停
    PauseStatePositionCooldown                // 仓位冷却暂停
)
```

---

### ✅ 3. 仓位监控与自动冷却

**状态**: 已完成

**目标**: 吃单后马上平单，然后进入冷静期暂停开单一段时间

**问题**: 限价单被吃单后会持有仓位，需要及时平仓并避免频繁开单累积风险

**实现方案**: 已实现 `MonitorPositions` 方法（集成在 `pkg/strategy/marketmaker.go`）

```go
// pkg/strategy/marketmaker.go
func (mm *MarketMaker) MonitorPositions(
    ctx context.Context,
    apiClient *client.Client,
    checkInterval time.Duration,   // 检查间隔
    cooldownDuration time.Duration, // 冷却时长
)
```

**工作流程**:
```
1. 定期通过 GET /api/query_positions 查询当前仓位
2. 检测到开放仓位（status=open, size!=0）:
   a. 调用 mm.Pause() → 自动取消所有订单并平仓
   b. 进入冷却期（不再开新单）
   c. 冷却期结束后调用 mm.Resume() 恢复做市
3. 冷却期内跳过仓位检查
```

**核心逻辑**:
```go
func (mm *MarketMaker) MonitorPositions(...) {
    ticker := time.NewTicker(checkInterval)
    var cooldownTimer *time.Timer
    var inCooldown bool

    for {
        select {
        case <-ticker.C:
            if inCooldown {
                continue  // 冷却期内跳过检查
            }

            // 查询仓位
            positions, err := apiClient.QueryPositions(mm.symbol)

            // 检查是否有开放仓位
            hasOpenPosition := false
            for _, pos := range positions {
                if pos.Status == "open" {
                    size, _ := parsePositionSize(pos.Qty)
                    if size != 0 {
                        hasOpenPosition = true
                        break
                    }
                }
            }

            if hasOpenPosition {
                // 调用 Pause 会自动取消订单并平仓
                mm.Pause(apiClient, PauseStatePositionCooldown)
                inCooldown = true

                // 设置冷却定时器
                cooldownTimer = time.AfterFunc(cooldownDuration, func() {
                    mm.Resume(PauseStatePositionCooldown)
                    inCooldown = false
                })
            }
        }
    }
}
```

**集成位置**:
- `pkg/client/client.go` - `QueryPositions` API 方法（返回所有仓位数组）
- `pkg/strategy/marketmaker.go` - `MonitorPositions` 方法
- `pkg/config/config.go` - `PositionCooldown` 配置结构体
- `cmd/main.go` - 启动监控 goroutine

**配置示例**:
```yaml
position_cooldown:
  enabled: true              # 是否启用仓位监控
  check_interval: 10s        # 检查间隔
  cooldown_duration: 30s     # 冷却时长
```

**功能特性**:
- ✅ 定期查询仓位状态（通过 `/api/query_positions` API）
- ✅ 检测到开放仓位立即调用 `mm.Pause()`（复用现有暂停机制）
- ✅ `mm.Pause()` 自动处理：取消所有订单 + 平仓
- ✅ 冷却期内不进行仓位检查
- ✅ 冷却期结束后自动恢复做市
- ✅ 使用新的 `PauseStatePositionCooldown` 状态

**API 接口**:
```
GET /api/query_positions?symbol=BTC-USD

Response:
[
  {
    "id": 15,
    "symbol": "BTC-USD",
    "qty": "0.940",
    "status": "open",
    "entry_price": "121737.96",
    "mark_price": "121715.05",
    ...
  }
]
```

**关键点**:
- 复用 `mm.Pause()` 方法，无需重复实现取消订单和平仓逻辑
- 冷却期避免频繁检查和平仓
- 使用 `time.AfterFunc` 实现异步恢复

---

## TODO 优化项

### 3. API 调用优化

**问题**: 每次 `UpdateOrders` 需要调用 2 次 `GetOpenOrdersByStatus`（bid + ask 各一次）

**优化方案**:
```go
// 当前实现: 每次更新调用 2 次 API
func (m *Manager) UpdateOrders(newBid, newAsk float64) error {
    m.cancelBidOrder()   // 调用 GetOpenOrdersByStatus
    m.cancelAskOrder()   // 再次调用 GetOpenOrdersByStatus
    // ...
}

// 优化后: 只调用 1 次 API
func (m *Manager) UpdateOrders(newBid, newAsk float64) error {
    // 一次性查询所有 open 订单
    orders, _ := m.client.GetOpenOrdersByStatus(m.symbol, "open")

    // 从缓存中找到 bid 和 ask 订单
    bidClOrdID := findOrderBySide(orders, "buy")
    askClOrdID := findOrderBySide(orders, "sell")

    // 批量取消
    if bidClOrdID != "" { m.client.CancelOrder(bidClOrdID) }
    if askClOrdID != "" { m.client.CancelOrder(askClOrdID) }
    // ...
}
```

**收益**: API 调用减少 50%

**实现位置**:
- `pkg/order/manager.go` - `UpdateOrders` 方法

---

### 4. 智能撤单策略

**问题**: 当挂单价差较大（10-30 bps）时，频繁撤单重挂增加成本

**优化策略**:
```go
type SmartCancelStrategy struct {
    maxSpreadBPS int  // 最大允许价差（10 bps）
    minPriceChangeBPS int  // 最小价格变化阈值（1 bps）
}

func (s *SmartCancelStrategy) ShouldUpdateOrder(
    currentOrderPrice float64,
    marketPrice float64,
) bool {
    // 计算挂单价与市价的价差
    spreadBPS := calculateSpreadBPS(currentOrderPrice, marketPrice)

    // 如果价差 < 10 bps，不需要更新（已经在范围内）
    if spreadBPS < s.maxSpreadBPS {
        return false
    }

    // 如果价差 >= 10 bps，需要更新
    return true
}
```

**逻辑**:
```
当前挂单价: 50000, 市价: 50010, 价差: 10 bps
→ 不撤单，等待市价接近

当前挂单价: 50000, 市价: 50050, 价差: 50 bps
→ 撤单重挂，保持价差在 10 bps 内
```

**集成位置**:
- `pkg/strategy/marketmaker.go` - `shouldUpdateOrders` 方法
- 配置文件添加 `smart_cancel` 配置

---

### 5. JWT Token 过期处理

**问题**: 当前使用 JWT 认证，但代码没有处理 token 过期场景

**现状**:
```go
// 当前实现: token 永久使用，不检查过期
apiClient.SetToken(loginResp.Token)  // 假设 token 永不过期

// 如果 token 过期，API 调用会返回 401 错误
resp, err := c.doRequest("POST", "/api/new_order", payload, signatures)
if resp.StatusCode == 401 {
    // 当前没有处理逻辑
}
```

**实现方案**:
```go
type TokenManager struct {
    token         string
    expiresAt     time.Time
    refreshBefore time.Duration  // 提前 5 分钟刷新
    authMgr       *auth.Auth
    chain         auth.Chain
    address       string
    signFn        auth.SignFunc
    mu            sync.RWMutex
}

func (tm *TokenManager) GetToken() (string, error) {
    tm.mu.RLock()
    if time.Until(tm.expiresAt) > tm.refreshBefore {
        token := tm.token
        tm.mu.RUnlock()
        return token, nil
    }
    tm.mu.RUnlock()

    // Token 即将过期或已过期，刷新
    return tm.refreshToken()
}

func (tm *TokenManager) refreshToken() (string, error) {
    tm.mu.Lock()
    defer tm.mu.Unlock()

    loginResp, err := tm.authMgr.Authenticate(tm.chain, tm.address, tm.signFn)
    if err != nil {
        return "", err
    }

    tm.token = loginResp.Token
    // 根据 JWT claims 解析过期时间
    tm.expiresAt = parseTokenExpiry(loginResp.Token)

    return tm.token, nil
}
```

**API 调用拦截**:
```go
func (c *Client) doRequest(method, path string, body []byte, signatures map[string]string) (*http.Response, error) {
    resp, err := c.httpClient.Do(req)

    // 检查 401 错误
    if resp.StatusCode == 401 {
        // Token 过期，刷新后重试
        if c.tokenMgr != nil {
            newToken, _ := c.tokenMgr.GetToken()
            c.SetToken(newToken)
            signatures = c.auth.SignRequest(body, reqID, timestamp)
            req.Header.Set("Authorization", "Bearer "+newToken)
            // 重试请求
            return c.httpClient.Do(req)
        }
    }

    return resp, err
}
```

**集成位置**:
- `pkg/client/client.go` - 添加 `TokenManager` 依赖
- `pkg/auth/` - 添加 `parseTokenExpiry()` 函数
- `cmd/main.go` - 使用 `TokenManager` 替代直接 `SetToken`

**优先级**: **高** - 生产环境必需

---

### 6. 定期平仓检查

> **✅ 已完成**: 此功能已与 TODO 7 合并为 **TODO 3: 仓位监控与自动冷却**（见上文）

**目标**: 定期检查用户的 position（而不是 open order），如果存在 position，则取消所有订单，然后市价平仓，等待重新开单

**问题**: 异常情况下（如网络中断、程序崩溃、SL/TP 失效），position 可能无法正常平仓，导致仓位暴露风险

**实现方案**:
```go
type PositionCleaner struct {
    client      *client.Client
    symbol      string
    interval    time.Duration  // 检查间隔（30 秒）
    stopCh      chan struct{}
}

func (pc *PositionCleaner) Start() {
    ticker := time.NewTicker(pc.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            pc.checkAndClosePositions()
        case <-pc.stopCh:
            return
        }
    }
}

func (pc *PositionCleaner) checkAndClosePositions() {
    // 查询当前 position
    pos, err := pc.client.GetPosition(pc.symbol)
    if err != nil {
        slog.Error("query position failed", "error", err)
        return
    }

    // 检查是否有未平仓位
    size, _ := strconv.ParseFloat(pos.Size, 64)
    if size == 0 {
        // 没有仓位，正常
        return
    }

    slog.Warn("detected open position, canceling all orders and closing position",
        "size", size,
        "side", pos.Side,
        "entry_price", pos.EntryPrice)

    // 1. 取消所有订单（避免新的订单成交）
    if err := pc.cancelAllOrders(); err != nil {
        slog.Error("cancel all orders failed", "error", err)
        return
    }

    // 2. 市价平仓
    closeReq := &client.NewOrderRequest{
        Symbol:      pc.symbol,
        Side:        getCloseSide(size),  // 根据仓位正负决定平仓方向
        OrderType:   client.OrderTypeMarket,
        Qty:         formatQty(math.Abs(size)),
        TimeInForce: client.TimeInForceIOC,
        ReduceOnly:  true,  // 只平仓
    }

    if _, err := pc.client.NewOrder(closeReq); err != nil {
        slog.Error("close position with market price failed", "error", err)
    } else {
        slog.Info("position closed with market price, waiting to resume market making",
            "closed_size", size)
    }
}

func (pc *PositionCleaner) cancelAllOrders() error {
    // 查询所有 open 订单
    orders, err := pc.client.GetOpenOrdersByStatus(pc.symbol, "new")
    if err != nil {
        return err
    }

    // 批量取消
    for _, ord := range orders {
        if err := pc.client.CancelOrder(ord.ClOrdID); err != nil {
            slog.Error("cancel order failed", "cl_ord_id", ord.ClOrdID, "error", err)
        }
    }

    return nil
}

func (pc *PositionCleaner) Stop() {
    close(pc.stopCh)
}

// 根据仓位正负决定平仓方向
// size > 0 (多头) -> 需要卖出平仓 -> "ask"
// size < 0 (空头) -> 需要买入平仓 -> "bid"
func getCloseSide(size float64) string {
    if size > 0 {
        return "ask"  // 平多头：卖出
    }
    return "bid"  // 平空头：买入
}
```

**配置示例**:
```yaml
position_cleaner:
  enabled: true          # 是否启用
  interval: 30s          # 检查间隔
```

**集成位置**:
- 新建 `pkg/position/cleaner.go`
- `cmd/main.go` - 启动时启动 cleaner goroutine，退出时停止

**逻辑说明**:
```
1. 每 30 秒检查一次 position（不是 open order）
2. 如果 position.size != 0（有未平仓位）：
   a. 取消所有 open orders（防止新的订单成交）
   b. 使用市价单平仓（reduce_only=true）
   c. 等待下次价格更新时重新开单
3. 记录日志便于追踪
```

**关键变化**:
- ✅ 检查 position 而不是 open order
- ✅ 先取消所有订单，再平仓（避免平仓期间新单成交）
- ✅ 平仓后等待下次价格更新自动恢复做市

---

**竞态条件 (Race Condition) 风险与解决方案**:

实现此功能时需要注意以下竞态条件场景：

**场景 1: Position Cleaner 自身重入**
```
时刻 T0: 第一次检查发现 position = 1 BTC
时刻 T1: 开始取消订单
时刻 T2: 平仓请求发送中（网络延迟）
时刻 T3: 第二次检查（30s后）→ 仍然看到 position = 1 BTC（API 延迟）
时刻 T4: 又发起新的取消 + 平仓请求 ❌
```

**场景 2: Position Cleaner 与主做市循环**
```
时刻 T0: Position Cleaner 检测到仓位，开始取消订单
时刻 T1: WebSocket 收到价格更新
时刻 T2: 主循环调用 UpdateOrders() → 创建新订单
时刻 T3: Position Cleaner 发送平仓请求
时刻 T4: 新订单又被吃掉，仓位增加 ❌
```

**场景 3: 检查 Position 与 SL/TP 触发**
```
时刻 T0: 检查 position = 1 BTC
时刻 T1: SL/TP 触发，平仓成功
时刻 T2: Position Cleaner 取消所有订单（此时已无仓位）
时刻 T3: Position Cleaner 发送平仓请求（reduce_only=true，但仓位为0）
```

**解决方案（使用 CAS 原子操作）**:

```go
import "sync/atomic"

type PositionCleaner struct {
    client      *client.Client
    symbol      string
    interval    time.Duration
    stopCh      chan struct{}

    // 使用 atomic 操作防止竞态
    isClosing   int32         // 0 = false, 1 = true
    lastCloseAt time.Time     // 上次平仓完成时间
}

func (pc *PositionCleaner) checkAndClosePositions() {
    // CAS 操作: 尝试将 isClosing 从 0 设置为 1
    // 如果失败，说明其他 goroutine 正在平仓，直接返回
    if !atomic.CompareAndSwapInt32(&pc.isClosing, 0, 1) {
        slog.Debug("already closing position, skip this check")
        return
    }
    defer atomic.StoreInt32(&pc.isClosing, 0)  // 完成后清除标记

    // 查询当前 position
    pos, err := pc.client.GetPosition(pc.symbol)
    if err != nil {
        slog.Error("query position failed", "error", err)
        return
    }

    // 检查是否有未平仓位
    size, _ := strconv.ParseFloat(pos.Size, 64)
    if size == 0 {
        return
    }

    slog.Warn("detected open position, canceling all orders and closing position",
        "size", size)

    // 1. 取消所有订单
    if err := pc.cancelAllOrders(); err != nil {
        slog.Error("cancel all orders failed", "error", err)
    }

    // 2. 市价平仓
    closeReq := &client.NewOrderRequest{
        Symbol:      pc.symbol,
        Side:        getCloseSide(size),
        OrderType:   client.OrderTypeMarket,
        Qty:         formatQty(math.Abs(size)),
        TimeInForce: client.TimeInForceIOC,
        ReduceOnly:  true,
    }

    if _, err := pc.client.NewOrder(closeReq); err != nil {
        slog.Error("close position failed", "error", err)
    } else {
        slog.Info("position closed", "size", size)
    }
}

// 提供给主循环调用，检查是否正在平仓
func (pc *PositionCleaner) IsClosing() bool {
    return atomic.LoadInt32(&pc.isClosing) == 1
}
```

**主循环集成**:

```go
func (mm *MarketMaker) OnPriceUpdate(data ws.PriceData) {
    // 检查是否正在平仓，如果是则跳过本次更新
    if mm.positionCleaner.IsClosing() {
        slog.Debug("position cleaner is closing, skip order update")
        return
    }

    // 正常做市逻辑
    newBid, newAsk := mm.calculatePrices(markPrice)
    if err := mm.orderMgr.UpdateOrders(newBid, newAsk); err != nil {
        slog.Error("update orders failed", "error", err)
    }
}
```

**CAS (Compare-And-Swap) 说明**:

CAS 是一种原子操作，包含三个参数：
- **内存值 (V)**: 当前内存中的值
- **预期旧值 (A)**: 认为应该是的值
- **新值 (B)**: 想要设置的新值

操作逻辑：如果内存值 V 等于预期值 A，则将 V 更新为 B，否则不做任何操作。整个过程是原子的。

```go
// 尝试将 isClosing 从 0 设置为 1
swapped := atomic.CompareAndSwapInt32(&pc.isClosing, 0, 1)
// 如果 swapped == true，成功设置
// 如果 swapped == false，说明当前值不是 0（已经在平仓中）
```

**额外优化建议**:

1. **增加冷却时间**: 平仓后等待 5-10 秒再允许下次检查
2. **添加超时机制**: 如果平仓超过 1 分钟未完成，强制重置 `isClosing`
3. **使用 Mutex 替代 CAS**: 如果临界区逻辑复杂，Mutex 更直观

**优先级**: **高** - 风险控制必需

---

### 7. 吃单后暂停开单

> **✅ 已完成**: 此功能已与 TODO 6 合并为 **TODO 3: 仓位监控与自动冷却**（见上文）

**目标**: 当限价单被吃单（成交）后，暂停开新的限价单，等待止盈/止损触发或手动平仓后恢复

**问题**: 当前实现在订单成交后会继续开新单，导致仓位累积，增加风险

**场景分析**:
```
正常情况:
1. bid 单被吃 → 持有多头仓位
2. 此时继续开新 bid/ask 单 → 仓位可能累积到 2 BTC、3 BTC...
3. 如果价格继续波动，风险敞口越来越大

期望行为:
1. bid 单被吃 → 持有多头仓位
2. 暂停开新单，等待 SL/TP 触发
3. TP/SL 触发后仓位清零 → 恢复做市
```

**实现方案**:
```go
type PositionState struct {
    hasPosition bool    // 是否持有仓位
    positionQty float64 // 当前仓位数量
    side        string  // "long" | "short"
    lastFillAt  time.Time
    mu          sync.RWMutex
}

func (ps *PositionState) HasOpenPosition() bool {
    ps.mu.RLock()
    defer ps.mu.RUnlock()
    return ps.hasPosition
}

func (ps *PositionState) MarkFilled(qty float64, side string) {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    ps.hasPosition = true
    ps.positionQty = qty
    ps.side = side
    ps.lastFillAt = time.Now()
    slog.Warn("order filled, pausing new orders",
        "qty", qty,
        "side", side,
        "waiting_for_sl_tp", "true")
}

func (ps *PositionState) MarkClosed() {
    ps.mu.Lock()
    defer ps.mu.Unlock()
    if ps.hasPosition {
        slog.Info("position closed, resuming market making",
            "held_duration", time.Since(ps.lastFillAt))
    }
    ps.hasPosition = false
    ps.positionQty = 0
    ps.side = ""
}
```

**策略集成**:
```go
// pkg/strategy/marketmaker.go

func (mm *MarketMaker) OnPriceUpdate(data ws.PriceData) {
    // 检查是否有未平仓位
    if mm.positionState.HasOpenPosition() {
        slog.Debug("skipping order placement: position still open",
            "waiting_for", "sl/tp to close")
        return
    }

    // 没有仓位，正常做市
    newBid, newAsk := mm.calculatePrices(markPrice)
    if err := mm.orderMgr.UpdateOrders(newBid, newAsk); err != nil {
        slog.Error("update orders failed", "error", err)
    }
}
```

**检测订单成交**:
```go
// 方案 1: 轮询仓位（最简单）
func (mm *MarketMaker) checkPosition() {
    pos, err := mm.client.GetPosition(mm.symbol)
    if err != nil {
        return
    }

    if pos != nil && pos.Size != "0" {
        size, _ := strconv.ParseFloat(pos.Size, 64)
        if size > 0 {
            mm.positionState.MarkFilled(size, "long")
        } else {
            mm.positionState.MarkFilled(-size, "short")
        }
    } else {
        mm.positionState.MarkClosed()
    }
}

// 方案 2: WebSocket 订单状态更新（推荐）
func (m *Manager) OnOrderUpdate(data ws.OrderDetail) {
    // 检测订单状态变为 filled 或 partially_filled
    if data.Status == "filled" || data.Status == "partially_filled" {
        fillQty, _ := strconv.ParseFloat(data.FillQty, 64)
        if fillQty > 0 {
            side := "long"
            if data.Side == "sell" {
                side = "short"
            }
            mm.positionState.MarkFilled(fillQty, side)
        }
    }
}
```

**配置示例**:
```yaml
position_guard:
  enabled: true
  check_interval: 10s    # 仓位检查间隔
  max_position_age: 5m   # 最大持仓时间（超时强制平仓）
```

**集成位置**:
- `pkg/strategy/position_state.go` - 仓位状态管理
- `pkg/strategy/marketmaker.go` - 在 `OnPriceUpdate` 中检查仓位
- `cmd/main.go` - 启动仓位检查 goroutine

**优先级**: **高** - 核心风险控制

---

### 优先级建议

| TODO | 状态 | 优先级 | 预计工作量 | 收益 |
|------|------|--------|-----------|------|
| 1. 时间窗口控制 | ✅ 已完成 | 中 | 2-3 小时 | 避开高风险时段 |
| 2. 波动保护机制 | ✅ 已完成 | 高 | 2-3 小时 | 防止异常波动损失 |
| 3. 仓位监控与冷却 | ✅ 已完成 | 高 | 2-3 小时 | 风险控制必需 |
| 4. API 调用优化 | 待开始 | 高 | 1 小时 | 减少 API 调用 50% |
| 5. 智能撤单策略 | 待开始 | 中 | 3-4 小时 | 降低交易成本 |
| 6. JWT Token 过期处理 | 待开始 | 高 | 2-3 小时 | 生产环境必需 |
| 7. 窗口期bug | 待开始 | 高 | 2-3 小时 | 生产环境必需 |
| 8. 除了websocket, 主动通过http接口fetch价格 保证高波动停止后能恢复 | 待开始 | 高 | 2-3 小时 | 生产环境必需 |

## 参考文档

- [StandX Perps Auth](https://docs.standx.com/standx-api/perps-auth)
- [StandX Perps WebSocket API](https://docs.standx.com/standx-api/perps-ws)
- [Market Maker Rules](https://docs.standx.com/docs/stand-x-campaigns/market-maker-uptime-program)
- [StandX Perps HTTP API](https://docs.standx.com/standx-api/perps-http-api)

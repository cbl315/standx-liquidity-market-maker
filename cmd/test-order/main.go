package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/spf13/cobra"
)

var (
	symbol  string
	chain   string
	apiURL  string
	price   float64
	qty     float64
	slTpBps int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "test-order",
		Short: "Test create and cancel order with HTTP query",
		Long: `Create an order WITHOUT client order ID (platform auto-generates),
query cl_ord_id via HTTP API, then cancel it using that ID.`,
		Run: runTestOrder,
	}

	rootCmd.Flags().StringVar(&symbol, "symbol", "BTC-USD", "Trading symbol (default: BTC-USD)")
	rootCmd.Flags().StringVar(&chain, "chain", "bsc", "Blockchain: bsc or solana (default: bsc)")
	rootCmd.Flags().StringVar(&apiURL, "api-url", "", "StandX Perps API URL (default: https://perps.standx.com)")
	rootCmd.Flags().Float64Var(&price, "price", 50000, "Order price (default: 50000)")
	rootCmd.Flags().Float64Var(&qty, "qty", 0.01, "Order quantity (default: 0.01)")
	rootCmd.Flags().IntVar(&slTpBps, "sl-tp-bps", 2, "Stop-loss/take-profit basis points (default: 2)")

	// 设置日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func runTestOrder(cmd *cobra.Command, args []string) {
	// 设置默认 URL
	if apiURL == "" {
		apiURL = "https://perps.standx.com"
	}

	// 获取钱包私钥
	privateKey := os.Getenv("WALLET_PRIVATE_KEY")
	if privateKey == "" {
		slog.Error("WALLET_PRIVATE_KEY environment variable is required")
		os.Exit(1)
	}

	// 创建钱包签名器
	wallet, err := auth.NewWalletSigner(privateKey)
	if err != nil {
		slog.Error("create wallet signer failed", "error", err)
		os.Exit(1)
	}

	slog.Info("wallet loaded", "address", wallet.Address().Hex())

	// 创建认证管理器
	authMgr := auth.NewAuth("https://api.standx.com")

	// 执行认证
	loginResp, err := authMgr.Authenticate(
		auth.Chain(chain),
		wallet.Address().Hex(),
		wallet.SignMessage,
	)
	if err != nil {
		slog.Error("authentication failed", "error", err)
		os.Exit(1)
	}

	slog.Info("authentication successful",
		"address", loginResp.Address,
		"alias", loginResp.Alias)

	// 创建 API 客户端
	apiClient := client.NewClient(apiURL, authMgr)
	apiClient.SetToken(loginResp.Token)

	// 计算 SL/TP 价格
	offset := price * float64(slTpBps) / 10000.0
	slPrice := price - offset
	tpPrice := price + offset

	// 创建订单请求（不传 ClOrdID，由平台生成）
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

	slog.Info("creating order (without cl_ord_id, platform will generate)",
		"side", "bid",
		"price", price,
		"sl_price", slPrice,
		"tp_price", tpPrice,
		"qty", qty)

	// 打印请求详情
	reqJSON, _ := json.MarshalIndent(req, "", "  ")
	fmt.Println("\n=== Order Request ===")
	fmt.Println(string(reqJSON))
	fmt.Println("=====================")

	// 下单
	resp, err := apiClient.NewOrder(req)
	if err != nil {
		slog.Error("create order failed", "error", err)
		os.Exit(1)
	}

	// 打印响应
	respJSON, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println("=== Order Response ===")
	fmt.Println(string(respJSON))
	fmt.Println("======================")

	slog.Info("order created successfully",
		"order_id", resp.OrderID(),
		"code", resp.Code,
		"message", resp.Message)

	// 等待一小段时间后查询订单
	fmt.Println("\n⏳ Waiting 2 seconds before querying order...")
	time.Sleep(2 * time.Second)

	// 通过 HTTP API 查询 open 订单获取 cl_ord_id
	fmt.Println("\n🔍 Querying open orders to get cl_ord_id...")
	orders, err := apiClient.GetOpenOrdersByStatus(symbol, "open")
	if err != nil {
		slog.Error("query open orders failed", "error", err)
		os.Exit(1)
	}

	// 找到我们刚创建的 buy side 订单
	var clOrdID string
	for _, ord := range orders {
		if ord.Side == "buy" {
			clOrdID = ord.ClOrdID
			fmt.Printf("\n✅ Found ClOrdID from HTTP API: %s\n", clOrdID)
			fmt.Printf("   Order ID: %d\n", ord.ID)
			fmt.Printf("   Status: %s\n", ord.Status)
			fmt.Printf("   Price: %s\n", ord.Price)
			fmt.Printf("   Qty: %s\n", ord.Qty)
			break
		}
	}

	if clOrdID == "" {
		slog.Error("no buy order found in open orders")
		os.Exit(1)
	}

	slog.Info("found cl_ord_id via HTTP API query", "cl_ord_id", clOrdID)

	// 等待一小段时间后取消订单
	fmt.Println("\n⏳ Waiting 2 seconds before cancelling...")
	time.Sleep(2 * time.Second)

	// 取消订单
	fmt.Printf("\n🔄 Cancelling order with cl_ord_id: %s\n", clOrdID)
	if err := apiClient.CancelOrder(clOrdID); err != nil {
		slog.Error("cancel order failed", "error", err)
		os.Exit(1)
	}

	slog.Info("order cancelled successfully", "cl_ord_id", clOrdID)
	fmt.Println("✅ Order cancelled successfully!")
}

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/spf13/cobra"
)

var (
	side     string
	price    float64
	qty      float64
	slTpBps  int
	symbol   string
	chain    string
	apiURL   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "create-order",
		Short: "Create an order with stop-loss and take-profit",
		Long: `Create a limit order with automatic stop-loss and take-profit prices.
The SL/TP prices are calculated based on the order price and the specified basis points.

Example:
  create-order --side=buy --price=50000 --qty=0.01 --sl-tp-bps=2
  create-order --side=sell --price=51000 --qty=0.01 --sl-tp-bps=1`,
		Run: runCreateOrder,
	}

	rootCmd.Flags().StringVar(&side, "side", "", "Order side: buy or sell (required)")
	rootCmd.Flags().Float64Var(&price, "price", 0, "Order price (required)")
	rootCmd.Flags().Float64Var(&qty, "qty", 0, "Order quantity (required)")
	rootCmd.Flags().IntVar(&slTpBps, "sl-tp-bps", 2, "Stop-loss/take-profit basis points (default: 2)")
	rootCmd.Flags().StringVar(&symbol, "symbol", "BTC-USD", "Trading symbol (default: BTC-USD)")
	rootCmd.Flags().StringVar(&chain, "chain", "bsc", "Blockchain: bsc or solana (default: bsc)")
	rootCmd.Flags().StringVar(&apiURL, "api-url", "", "StandX API URL (default: https://perps.standx.com)")

	rootCmd.MarkFlagRequired("side")
	rootCmd.MarkFlagRequired("price")
	rootCmd.MarkFlagRequired("qty")

	// 设置日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func runCreateOrder(cmd *cobra.Command, args []string) {
	// 验证 side
	var orderSide client.OrderSide
	switch side {
	case "buy":
		orderSide = client.OrderBid
	case "sell":
		orderSide = client.OrderAsk
	default:
		slog.Error("invalid side", "side", side, "expected", "buy or sell")
		os.Exit(1)
	}

	// 设置默认 API URL
	if apiURL == "" {
		apiURL = "https://perps.standx.com"
	}

	slog.Info("creating order with SL/TP",
		"side", side,
		"price", price,
		"qty", qty,
		"sl_tp_bps", slTpBps,
		"symbol", symbol)

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
	slOffset := price * float64(slTpBps) / 10000.0
	var slPrice, tpPrice float64

	switch orderSide {
	case client.OrderBid:
		// Bid 单（买入）：止损更低，止盈更高
		slPrice = price - slOffset
		tpPrice = price + slOffset
	case client.OrderAsk:
		// Ask 单（卖出）：止损更高，止盈更低
		slPrice = price + slOffset
		tpPrice = price - slOffset
	}

	slog.Info("calculated SL/TP prices",
		"order_price", price,
		"sl_price", slPrice,
		"tp_price", tpPrice,
		"sl_offset", slOffset)

	// 创建订单请求
	req := &client.NewOrderRequest{
		Symbol:      symbol,
		Side:        orderSide,
		OrderType:   client.OrderTypeLimit,
		Qty:         client.FormatQty(qty),
		Price:       client.FormatPrice(price),
		SlPrice:     client.FormatPrice(slPrice),
		TpPrice:     client.FormatPrice(tpPrice),
		TimeInForce: client.TimeInForceGTC,
		ReduceOnly:  false,
	}

	// 打印请求详情
	reqJSON, _ := json.MarshalIndent(req, "", "  ")
	fmt.Println("=== Order Request ===")
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
}

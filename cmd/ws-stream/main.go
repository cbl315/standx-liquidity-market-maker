package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg struct {
	Symbol     string
	APIBaseURL string // 用于认证
	WSURL      string // WebSocket URL (统一端点)
	Chain      string
	PrivateKey string
	Verbose    bool
}

func main() {
	// 配置日志
	setupLogger()

	rootCmd := &cobra.Command{
		Use:   "ws-stream",
		Short: "Stream price and order updates via WebSocket",
		Long:  `Connect to StandX WebSocket API to stream real-time price and order updates.`,
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&cfg.Symbol, "symbol", "s", "BTC-USD", "Trading pair symbol (e.g., BTC-USD, ETH-USD)")
	rootCmd.Flags().StringVarP(&cfg.APIBaseURL, "api-url", "a", "https://api.standx.com", "API base URL for authentication")
	rootCmd.Flags().StringVarP(&cfg.WSURL, "ws-url", "w", "wss://perps.standx.com/ws-stream/v1", "WebSocket URL (default: wss://perps.standx.com/ws-stream/v1)")
	rootCmd.Flags().StringVarP(&cfg.Chain, "chain", "c", "bsc", "Blockchain network (bsc, solana)")
	rootCmd.Flags().StringVarP(&cfg.PrivateKey, "private-key", "k", "", "Wallet private key (or set WALLET_PRIVATE_KEY env var)")
	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose logging (debug level)")

	// 绑定到 viper
	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		slog.Error("failed to bind flags", "error", err)
		os.Exit(1)
	}

	// 从环境变量读取 private key
	if cfg.PrivateKey == "" {
		cfg.PrivateKey = viper.GetString("wallet_private_key")
	}

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command execution failed", "error", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	// 如果设置了 verbose，重新配置日志级别
	if cfg.Verbose {
		opts := &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
		handler := slog.NewTextHandler(os.Stderr, opts)
		slog.SetDefault(slog.New(handler))
	}

	// 验证必需参数
	if cfg.Symbol == "" {
		slog.Error("symbol is required")
		_ = cmd.Usage()
		os.Exit(1)
	}

	if cfg.PrivateKey == "" {
		slog.Error("private-key is required (use --private-key flag or WALLET_PRIVATE_KEY env var)")
		_ = cmd.Usage()
		os.Exit(1)
	}

	// 创建钱包签名器
	wallet, err := auth.NewWalletSigner(cfg.PrivateKey)
	if err != nil {
		slog.Error("create wallet signer failed", "error", err)
		os.Exit(1)
	}

	slog.Info("wallet loaded", "address", wallet.Address().Hex())

	// 创建认证管理器
	authMgr := auth.NewAuth(cfg.APIBaseURL)

	// 执行认证
	slog.Info("authenticating...", "chain", cfg.Chain)
	loginResp, err := authMgr.Authenticate(
		auth.Chain(cfg.Chain),
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

	// 创建消息处理器
	handler := &StreamHandler{
		symbol: cfg.Symbol,
		mu:     sync.Mutex{},
		orders: make(map[string]*ws.OrderUpdateMessage),
	}

	// 创建 WebSocket 客户端 (统一端点)
	wsClient := ws.NewClient(cfg.WSURL)
	wsClient.SetMessageHandler(handler)

	slog.Info("connecting to WebSocket", "url", cfg.WSURL)
	if err := wsClient.Connect(loginResp.Token); err != nil {
		slog.Error("websocket connect failed", "error", err)
		os.Exit(1)
	}
	defer wsClient.Close()

	// 登录 WebSocket
	if err := wsClient.Login(); err != nil {
		slog.Error("websocket login failed", "error", err)
		os.Exit(1)
	}

	// 订阅价格 (public channel)
	if err := wsClient.SubscribePrice(cfg.Symbol); err != nil {
		slog.Error("subscribe price failed", "error", err)
		os.Exit(1)
	}
	slog.Info("subscribed to price updates", "symbol", cfg.Symbol)

	// 订阅用户订单 (authenticated channel)
	if err := wsClient.SubscribeUserOrders(); err != nil {
		slog.Error("subscribe orders failed", "error", err)
		os.Exit(1)
	}
	slog.Info("subscribed to order updates")

	// 订阅用户仓位 (authenticated channel)
	if err := wsClient.SubscribeUserPositions(); err != nil {
		slog.Error("subscribe positions failed", "error", err)
		os.Exit(1)
	}
	slog.Info("subscribed to position updates")

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("streaming started, press Ctrl+C to stop")
	fmt.Println("\n" + strings.Repeat("─", 80))
	fmt.Printf("Streaming %s\n", cfg.Symbol)
	fmt.Println(strings.Repeat("─", 80))

	<-sigCh
	slog.Info("shutting down...")

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("Final Summary")
	fmt.Println(strings.Repeat("=", 80))
	handler.printSummary()
}

// setupLogger 配置日志
func setupLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}

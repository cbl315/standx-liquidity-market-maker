package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg struct {
	Symbol      string
	APIBaseURL  string // 用于认证
	PerpsURL    string // 用于 HTTP API
	Chain       string
	PrivateKey  string
	Verbose     bool
}

func main() {
	// 配置日志
	setupLogger()

	rootCmd := &cobra.Command{
		Use:   "get-price",
		Short: "Get ticker price from StandX API",
		Long:  `Fetch the current mark price for a given trading pair symbol from the StandX API.`,
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&cfg.Symbol, "symbol", "s", "BTC-USD", "Ticker symbol (e.g., BTC-USD, ETH-USD)")
	rootCmd.Flags().StringVarP(&cfg.APIBaseURL, "api-url", "a", "https://api.standx.com", "API base URL for authentication (default: https://api.standx.com)")
	rootCmd.Flags().StringVarP(&cfg.PerpsURL, "perps-url", "p", "https://perps.standx.com", "Perps API URL for trading (default: https://perps.standx.com)")
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

	// 创建认证管理器 (使用 api_url)
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

	// 创建 API 客户端 (使用 perps_url)
	apiClient := client.NewClient(cfg.PerpsURL, authMgr)
	apiClient.SetToken(loginResp.Token)

	// 获取标记价格
	slog.Info("getting mark price", "symbol", cfg.Symbol)
	price, err := apiClient.GetMarkPrice(cfg.Symbol)
	if err != nil {
		slog.Error("failed to get mark price", "error", err)
		os.Exit(1)
	}

	// 输出价格
	fmt.Printf("%s: %.2f\n", cfg.Symbol, price)
}

// setupLogger 配置日志
func setupLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}

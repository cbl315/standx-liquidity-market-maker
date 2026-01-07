package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg struct {
	APIBaseURL string // 用于认证
	PerpsURL   string // 用于 HTTP API
	Chain      string
	PrivateKey string
	Verbose    bool
	Json       bool
}

func main() {
	// 配置日志
	setupLogger()

	rootCmd := &cobra.Command{
		Use:   "get-balance",
		Short: "Get account balance from StandX API",
		Long:  `Fetch the current account balance information from the StandX API.`,
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&cfg.APIBaseURL, "api-url", "a", "https://api.standx.com", "API base URL for authentication (default: https://api.standx.com)")
	rootCmd.Flags().StringVarP(&cfg.PerpsURL, "perps-url", "p", "https://perps.standx.com", "Perps API URL for trading (default: https://perps.standx.com)")
	rootCmd.Flags().StringVarP(&cfg.Chain, "chain", "c", "bsc", "Blockchain network (bsc, solana)")
	rootCmd.Flags().StringVarP(&cfg.PrivateKey, "private-key", "k", "", "Wallet private key (or set WALLET_PRIVATE_KEY env var)")
	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose logging (debug level)")
	rootCmd.Flags().BoolVarP(&cfg.Json, "json", "j", false, "Output in JSON format")

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

	// 获取账户余额
	slog.Info("getting account balance")
	balance, err := apiClient.GetBalance()
	if err != nil {
		slog.Error("failed to get balance", "error", err)
		os.Exit(1)
	}

	// 输出结果
	if cfg.Json {
		outputJSON(balance)
	} else {
		outputTable(balance)
	}
}

// outputJSON 输出 JSON 格式
func outputJSON(balance *client.BalanceResponse) {
	data, err := json.MarshalIndent(balance, "", "  ")
	if err != nil {
		slog.Error("failed to marshal json", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// outputTable 输出表格格式
func outputTable(balance *client.BalanceResponse) {
	fmt.Println("=== Account Balance ===")
	fmt.Printf("Balance:           %s\n", balance.Balance)
	fmt.Printf("Equity:            %s\n", balance.Equity)
	fmt.Printf("UPNL:              %s\n", balance.UPNL)
	fmt.Println()
	fmt.Println("--- Cross Margin ---")
	fmt.Printf("Cross Balance:     %s\n", balance.CrossBalance)
	fmt.Printf("Cross Available:   %s\n", balance.CrossAvailable)
	fmt.Printf("Cross Margin:      %s\n", balance.CrossMargin)
	fmt.Printf("Cross UPNL:        %s\n", balance.CrossUPNL)
	fmt.Println()
	fmt.Println("--- Isolated ---")
	fmt.Printf("Isolated Balance:  %s\n", balance.IsolatedBalance)
	fmt.Printf("Isolated UPNL:     %s\n", balance.IsolatedUPNL)
	fmt.Println()
	fmt.Println("--- Other ---")
	fmt.Printf("Locked:            %s\n", balance.Locked)
	fmt.Printf("PNL Freeze:        %s\n", balance.PNLFreeze)
	fmt.Println("======================")
}

// setupLogger 配置日志
func setupLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfg struct {
	APIBaseURL string
	PerpsURL   string
	Chain      string
	PrivateKey string
	Symbol     string
	Hours      int
	Verbose    bool
	Json       bool
}

func main() {
	setupLogger()

	rootCmd := &cobra.Command{
		Use:   "get-trades",
		Short: "Get and analyze trade history from StandX API",
		Long:  `Fetch trade history and calculate statistics (total fees, PNL, trade count).`,
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&cfg.APIBaseURL, "api-url", "a", "https://api.standx.com", "API base URL")
	rootCmd.Flags().StringVarP(&cfg.PerpsURL, "perps-url", "p", "https://perps.standx.com", "Perps API URL")
	rootCmd.Flags().StringVarP(&cfg.Chain, "chain", "c", "bsc", "Blockchain network (bsc, solana)")
	rootCmd.Flags().StringVarP(&cfg.PrivateKey, "private-key", "k", "", "Wallet private key")
	rootCmd.Flags().StringVarP(&cfg.Symbol, "symbol", "s", "BTC-USD", "Trading symbol")
	rootCmd.Flags().IntVarP(&cfg.Hours, "hours", "d", 24, "Hours of history to fetch (default: 24)")
	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose logging")
	rootCmd.Flags().BoolVarP(&cfg.Json, "json", "j", false, "Output in JSON format")

	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		slog.Error("failed to bind flags", "error", err)
		os.Exit(1)
	}

	if cfg.PrivateKey == "" {
		cfg.PrivateKey = viper.GetString("wallet_private_key")
	}

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command execution failed", "error", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	if cfg.Verbose {
		opts := &slog.HandlerOptions{Level: slog.LevelDebug}
		handler := slog.NewTextHandler(os.Stderr, opts)
		slog.SetDefault(slog.New(handler))
	}

	if cfg.PrivateKey == "" {
		slog.Error("private-key is required")
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

	// 创建 API 客户端
	apiClient := client.NewClient(cfg.PerpsURL, authMgr)
	apiClient.SetToken(loginResp.Token)

	// 计算时间范围
	end := time.Now()
	start := end.Add(-time.Duration(cfg.Hours) * time.Hour)

	slog.Info("fetching trades",
		"symbol", cfg.Symbol,
		"start", start.Format(time.RFC3339),
		"end", end.Format(time.RFC3339))

	// 获取成交记录
	tradesResp, err := apiClient.GetTrades(cfg.Symbol, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		slog.Error("failed to get trades", "error", err)
		os.Exit(1)
	}

	// 计算统计数据
	stats := calculateStats(tradesResp.Result)

	// 输出结果
	if cfg.Json {
		outputJSON(stats, tradesResp.Result)
	} else {
		outputTable(stats, tradesResp.Result)
	}
}

type Stats struct {
	TotalFee      *big.Float
	TotalPNL      *big.Float
	TradeCount    int
	Symbol        string
	TimeRange     string
	Hours         int
}

func calculateStats(trades []client.Trade) *Stats {
	stats := &Stats{
		TotalFee:   new(big.Float),
		TotalPNL:   new(big.Float),
		TradeCount: len(trades),
		Hours:      cfg.Hours,
		Symbol:     cfg.Symbol,
		TimeRange:  fmt.Sprintf("Past %d hours", cfg.Hours),
	}

	for _, trade := range trades {
		// 累加 fee
		fee := new(big.Float)
		fee.SetString(trade.FeeQty)
		stats.TotalFee.Add(stats.TotalFee, fee)

		// 累加 pnl
		pnl := new(big.Float)
		pnl.SetString(trade.PNL)
		stats.TotalPNL.Add(stats.TotalPNL, pnl)
	}

	return stats
}

func outputJSON(stats *Stats, trades []client.Trade) {
	type JSONOutput struct {
		Stats  Stats             `json:"stats"`
		Trades []client.Trade    `json:"trades,omitempty"`
	}

	output := JSONOutput{
		Stats:  *stats,
	}

	if cfg.Verbose {
		output.Trades = trades
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		slog.Error("failed to marshal json", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputTable(stats *Stats, trades []client.Trade) {
	fmt.Printf("=== Trade Statistics: %s ===\n", stats.Symbol)
	fmt.Printf("Time Range:        %s\n", stats.TimeRange)
	fmt.Printf("Trade Count:       %d\n", stats.TradeCount)
	fmt.Printf("Total Fees:        %s\n", stats.TotalFee.Text('f', 6))
	fmt.Printf("Total PNL:         %s\n", stats.TotalPNL.Text('f', 6))
	fmt.Println("============================")

	if cfg.Verbose && len(trades) > 0 {
		fmt.Println("\n--- Trade Details ---")
		for i, t := range trades {
			fmt.Printf("[%d] ID:%d OrderID:%d %s %s @ %s | Fee:%s %s PNL:%s\n",
				i+1, t.ID, t.OrderID, t.Side, t.Qty, t.Price, t.FeeQty, t.FeeAsset, t.PNL)
		}
	}
}

func setupLogger() {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}

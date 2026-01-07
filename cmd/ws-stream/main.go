package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
	"github.com/spf13/cobra"
)

var cfg struct {
	Symbol     string
	WSURL      string
	Verbose    bool
}

func main() {
	// 配置日志
	setupLogger()

	rootCmd := &cobra.Command{
		Use:   "ws-stream",
		Short: "Stream price updates via WebSocket",
		Long:  `Connect to StandX WebSocket API to stream real-time price updates.`,
		Run:   run,
	}

	rootCmd.Flags().StringVarP(&cfg.Symbol, "symbol", "s", "BTC-USD", "Trading pair symbol (e.g., BTC-USD, ETH-USD)")
	rootCmd.Flags().StringVarP(&cfg.WSURL, "ws-url", "w", "wss://perps.standx.com/ws-stream/v1", "WebSocket URL")
	rootCmd.Flags().BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose logging (debug level)")

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

	// 创建消息处理器
	handler := &StreamHandler{
		symbol:     cfg.Symbol,
		mu:         sync.Mutex{},
		lastPrice:  0,
		priceCount: 0,
		startTime:  time.Now(),
	}

	// 创建 WebSocket 客户端
	wsClient := ws.NewClient(cfg.WSURL)
	wsClient.SetPriceHandler(handler)

	slog.Info("connecting to WebSocket", "url", cfg.WSURL)
	if err := wsClient.Connect(); err != nil {
		slog.Error("websocket connect failed", "error", err)
		os.Exit(1)
	}
	defer wsClient.Close()

	// 启动重连监控
	go func() {
		for range wsClient.ReconnectChan() {
			slog.Warn("websocket disconnected, reconnecting...")
			for {
				if err := wsClient.Connect(); err != nil {
					slog.Error("websocket reconnect failed, retrying...", "error", err)
					time.Sleep(5 * time.Second)
					continue
				}
				// 重新订阅价格
				if err := wsClient.SubscribePrice(cfg.Symbol); err != nil {
					slog.Error("subscribe price after reconnect failed", "error", err)
					continue
				}
				slog.Info("websocket reconnected successfully")
				break
			}
		}
	}()

	// 订阅价格
	if err := wsClient.SubscribePrice(cfg.Symbol); err != nil {
		slog.Error("subscribe price failed", "error", err)
		os.Exit(1)
	}
	slog.Info("subscribed to price updates", "symbol", cfg.Symbol)

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

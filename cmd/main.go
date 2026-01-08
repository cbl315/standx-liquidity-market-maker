package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/config"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/order"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/risk"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/strategy"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/timewindow"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/volatility"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

func main() {
	// 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	// 配置日志
	setupLogger(cfg.Log)

	// 创建时间窗口过滤器
	windowFilter, err := timewindow.NewFilter(cfg.TimeWindow)
	if err != nil {
		slog.Error("create time window filter failed", "error", err)
		os.Exit(1)
	}

	slog.Info("starting StandX market maker",
		"chain", cfg.Chain,
		"symbol", cfg.Strategy.Symbol,
		"order_qty", cfg.Strategy.OrderQty,
		"spread_bps", cfg.Strategy.SpreadBPS,
		"sl_tp_bps", cfg.Risk.SlTpBPS)

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
	authMgr := auth.NewAuth(cfg.API.BaseURL)

	// 执行认证
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
	apiClient := client.NewClient(cfg.API.PerpsURL, authMgr)
	apiClient.SetToken(loginResp.Token)

	// 创建 WebSocket 客户端
	wsClient := ws.NewClient(cfg.WS.URL)

	if err := wsClient.Connect(); err != nil {
		slog.Error("websocket connect failed", "error", err)
		os.Exit(1)
	}
	defer wsClient.Close()

	// 启动 WebSocket 重连监控
	go func() {
		for range wsClient.ReconnectChan() {
			slog.Warn("websocket disconnected, reconnecting...")
			reconnectDelay := cfg.WS.ReconnectDelay
			if reconnectDelay == 0 {
				reconnectDelay = 5 * time.Second
			}
			for {
				if err := wsClient.Connect(); err != nil {
					slog.Error("websocket reconnect failed, retrying...", "error", err, "retry_after", reconnectDelay)
					time.Sleep(reconnectDelay)
					continue
				}
				// 重新订阅价格
				if err := wsClient.SubscribePrice(cfg.Strategy.Symbol); err != nil {
					slog.Error("subscribe price after reconnect failed", "error", err)
					time.Sleep(reconnectDelay)
					continue
				}
				slog.Info("websocket reconnected successfully")
				break
			}
		}
	}()

	// 订阅价格
	if err := wsClient.SubscribePrice(cfg.Strategy.Symbol); err != nil {
		slog.Error("subscribe price failed", "error", err)
		os.Exit(1)
	}
	slog.Info("subscribed to price updates", "symbol", cfg.Strategy.Symbol)

	// 创建订单管理器（带 SL/TP）
	orderMgr := order.NewManager(
		apiClient,
		cfg.Strategy.Symbol,
		cfg.Strategy.OrderQty,
		cfg.Risk.SlTpBPS,
	)

	// 创建风险管理器（简化版 - 只检查余额）
	riskMgr := risk.NewManager(
		apiClient,
		risk.RiskConfig{
			Enabled:         cfg.Risk.Enabled,
			SlTpBPS:         cfg.Risk.SlTpBPS,
			MinBalanceRatio: cfg.Risk.MinBalanceRatio,
		},
	)

	// 创建波动保护器
	var volGuard *volatility.Guard
	if cfg.Volatility.Enabled {
		volGuard = volatility.NewGuard(
			cfg.Volatility.ThresholdBPS,
			cfg.Volatility.WindowSec,
			cfg.Volatility.MinSnapshots,
		)
		slog.Info("volatility guard enabled",
			"threshold_bps", cfg.Volatility.ThresholdBPS,
			"window_sec", cfg.Volatility.WindowSec,
			"min_snapshots", cfg.Volatility.MinSnapshots)
	}

	// 创建做市策略
	mm := strategy.NewMarketMaker(
		orderMgr,
		riskMgr,
		wsClient,
		volGuard,
		cfg.Strategy.Symbol,
		cfg.Strategy.OrderQty,
		cfg.Strategy.SpreadBPS,
	)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动运行时时间窗口监控（仅当启用且action为shutdown时）
	if cfg.TimeWindow.Enabled && cfg.TimeWindow.Action == config.WindowActionShutdown {
		go mm.MonitorTimeWindow(ctx, windowFilter, apiClient, orderMgr)
	}

	// 启动运行时波动率监控
	if cfg.Volatility.Enabled {
		go mm.MonitorVolatility(ctx, apiClient, orderMgr)
	}

	// 启动仓位监控（吃单后马上平单，然后进入冷静期）
	if cfg.PositionCooldown.Enabled {
		go mm.MonitorPositions(ctx, apiClient, cfg.PositionCooldown.CheckInterval, cfg.PositionCooldown.CooldownDuration)
		slog.Info("position monitoring enabled",
			"check_interval", cfg.PositionCooldown.CheckInterval,
			"cooldown_duration", cfg.PositionCooldown.CooldownDuration)
	}

	// 启动做市策略
	go func() {
		if err := mm.Run(ctx); err != nil {
			slog.Error("market maker run failed", "error", err)
			cancel()
		}
	}()

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("market maker is running, press Ctrl+C to stop")

	<-sigCh
	slog.Info("shutting down...")

	// 取消上下文
	cancel()

	// 取消所有订单
	if err := orderMgr.CancelAll(); err != nil {
		slog.Error("cancel all orders failed", "error", err)
	}

	slog.Info("market maker stopped")
}

// setupLogger 配置日志
func setupLogger(logCfg config.LogConfig) {
	var level slog.Level
	switch logCfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if logCfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
	slog.Info("logger configured", "level", level, "format", logCfg.Format)
}

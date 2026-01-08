package config

import (
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config 应用配置
type Config struct {
	Chain      string         `mapstructure:"chain"`
	API        APIConfig      `mapstructure:"api"`
	Strategy   Strategy       `mapstructure:"strategy"`
	Risk       RiskConfig     `mapstructure:"risk"`
	WS         WSConfig       `mapstructure:"websocket"`
	Monitor    Monitor        `mapstructure:"monitor"`
	TimeWindow TimeWindowConfig `mapstructure:"time_window"`
	Log        LogConfig      `mapstructure:"log"`
}

// APIConfig API 配置
type APIConfig struct {
	BaseURL   string        `mapstructure:"base_url"`
	PerpsURL  string        `mapstructure:"perps_url"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

// Strategy 做市策略配置
type Strategy struct {
	Symbol    string  `mapstructure:"symbol"`
	OrderQty  float64 `mapstructure:"order_qty"`
	SpreadBPS int     `mapstructure:"spread_bps"`
}

// RiskConfig 风险管理配置
type RiskConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	SlTpBPS          int           `mapstructure:"sl_tp_bps"`
	MinBalanceRatio  float64       `mapstructure:"min_balance_ratio"`
	CheckInterval    time.Duration `mapstructure:"check_interval"`
}

// WSConfig WebSocket 配置
type WSConfig struct {
	URL            string        `mapstructure:"url"`
	PingInterval   time.Duration `mapstructure:"ping_interval"`
	PongTimeout    time.Duration `mapstructure:"pong_timeout"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`
}

// Monitor 监控配置
type Monitor struct {
	UptimeThreshold float64       `mapstructure:"uptime_threshold"`
	LogInterval     time.Duration `mapstructure:"log_interval"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// TimeWindowConfig 时间窗口配置
type TimeWindowConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	TimeZone string        `mapstructure:"timezone"`
	Windows  []WindowSpec  `mapstructure:"windows"`
	Action   WindowAction  `mapstructure:"action"`
}

// WindowSpec 时间窗口定义
type WindowSpec struct {
	Start    string `mapstructure:"start"`    // "22:00"
	End      string `mapstructure:"end"`      // "03:00"
	Weekdays []int  `mapstructure:"weekdays"` // [1,2,3,4,5]
}

// WindowAction 窗口期内执行的操作
type WindowAction string

const (
	WindowActionShutdown WindowAction = "shutdown" // 停止运行
	WindowActionPause    WindowAction = "pause"    // 暂停下单
)

// Load 从文件加载配置
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 设置配置文件
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// 读取环境变量
	v.AutomaticEnv()

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// 从环境变量覆盖私钥
	if pk := os.Getenv("WALLET_PRIVATE_KEY"); pk != "" {
		// 私钥不存储在配置中，单独处理
		_ = pk
	}

	return &cfg, nil
}

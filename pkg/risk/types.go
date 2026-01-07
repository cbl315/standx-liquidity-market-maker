package risk

// RiskConfig 风险配置
type RiskConfig struct {
	Enabled         bool    `json:"enabled"`
	SlTpBPS         int     `json:"sl_tp_bps"`          // SL/TP 价格浮动范围
	MinBalanceRatio float64 `json:"min_balance_ratio"`  // 最低余额比例
}

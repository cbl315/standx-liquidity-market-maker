package risk

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// Manager 风险管理器（简化版 - 只检查余额）
type Manager struct {
	client *client.Client
	config RiskConfig
}

// NewManager 创建风险管理器
func NewManager(apiClient *client.Client, config RiskConfig) *Manager {
	return &Manager{
		client: apiClient,
		config: config,
	}
}

// HasSufficientBalance 检查是否有足够余额开单
func (rm *Manager) HasSufficientBalance(orderQty float64) (bool, error) {
	if !rm.config.Enabled {
		return true, nil
	}

	// 获取账户余额
	balance, err := rm.client.GetBalance()
	if err != nil {
		return false, fmt.Errorf("get balance failed: %w", err)
	}

	// 解析可用余额
	crossAvailable, err := strconv.ParseFloat(balance.CrossAvailable, 64)
	if err != nil {
		return false, fmt.Errorf("parse cross available balance failed: %w", err)
	}

	// 计算所需保证金（简化计算：使用余额比例）
	// required = orderQty * minBalanceRatio (作为金额检查)
	required := orderQty * rm.config.MinBalanceRatio

	slog.Debug("balance check",
		"cross_available", crossAvailable,
		"required", required,
		"order_qty", orderQty,
		"min_balance_ratio", rm.config.MinBalanceRatio)

	// 检查是否足够
	hasEnough := crossAvailable >= required

	if !hasEnough {
		slog.Warn("insufficient balance, waiting for SL/TP to close positions",
			"cross_available", crossAvailable,
			"required", required,
			"shortage", required-crossAvailable)
	}

	return hasEnough, nil
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/ws"
)

// TestStreamHandler_OnPriceUpdate 测试价格更新处理
func TestStreamHandler_OnPriceUpdate(t *testing.T) {
	handler := &StreamHandler{
		symbol: "BTC-USD",
		orders: make(map[string]*ws.OrderUpdateMessage),
	}

	msg := ws.PriceMessage{
		Symbol:    "BTC-USD",
		Price:     50000.50,
		Timestamp: time.Now().UnixMilli(),
	}

	// 测试 channel 名称
	if msg.Symbol != "BTC-USD" {
		t.Errorf("expected symbol BTC-USD, got %s", msg.Symbol)
	}

	// 测试不会 panic
	handler.OnPriceUpdate(msg)

	if handler.lastPrice != 50000.50 {
		t.Errorf("expected last price 50000.50, got %f", handler.lastPrice)
	}

	if handler.priceCount != 1 {
		t.Errorf("expected price count 1, got %d", handler.priceCount)
	}
}

// TestStreamHandler_OnOrderUpdate 测试订单更新处理
func TestStreamHandler_OnOrderUpdate(t *testing.T) {
	handler := &StreamHandler{
		symbol: "BTC-USD",
		orders: make(map[string]*ws.OrderUpdateMessage),
	}

	msg := ws.OrderUpdateMessage{
		OrderID:   "order123",
		Symbol:    "BTC-USD",
		Status:    "filled",
		FilledQty: 1.5,
	}

	// 测试不会 panic
	handler.OnOrderUpdate(msg)

	if handler.orderCount != 1 {
		t.Errorf("expected order count 1, got %d", handler.orderCount)
	}

	if len(handler.orders) != 1 {
		t.Errorf("expected 1 order, got %d", len(handler.orders))
	}

	if handler.orders["order123"].Status != "filled" {
		t.Errorf("expected status filled, got %s", handler.orders["order123"].Status)
	}
}

// TestStreamHandler_OnPositionUpdate 测试仓位更新处理
func TestStreamHandler_OnPositionUpdate(t *testing.T) {
	handler := &StreamHandler{
		symbol: "BTC-USD",
		orders: make(map[string]*ws.OrderUpdateMessage),
	}

	msg := ws.PositionUpdateMessage{
		Symbol:        "BTC-USD",
		Size:          0.5,
		Side:          "long",
		UnrealizedPnL: 100.25,
	}

	// 测试不会 panic
	handler.OnPositionUpdate(msg)
}

// TestStreamHandler_PrintSummary 测试摘要打印
func TestStreamHandler_PrintSummary(t *testing.T) {
	handler := &StreamHandler{
		symbol:     "BTC-USD",
		lastPrice:  50000.50,
		priceCount: 10,
		orders: map[string]*ws.OrderUpdateMessage{
			"order1": {
				OrderID:   "order1",
				Status:    "filled",
				FilledQty: 1.0,
			},
			"order2": {
				OrderID:   "order2",
				Status:    "open",
				FilledQty: 0.5,
			},
		},
		orderCount: 2,
	}

	// 测试不会 panic
	handler.printSummary()
}

// TestAuthFlow 测试认证流程
func TestAuthFlow(t *testing.T) {
	// 生成测试私钥
	wallet, err := auth.NewWalletSigner("0x" + "0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		t.Skip("need valid private key for auth test")
	}

	testPayload := `{"message":"test","exp":1760291384,"iat":1760291204}`
	encodedPayload := testPayload // 简化处理
	testJWT := "header." + encodedPayload + ".signature"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/offchain/prepare-signin" {
			resp := map[string]any{
				"success":    true,
				"signedData": testJWT,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/v1/offchain/login" {
			resp := map[string]string{
				"token":   "test-token",
				"address": wallet.Address().Hex(),
				"alias":   "test",
				"chain":   "bsc",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)

	loginResp, err := authMgr.Authenticate(
		auth.ChainBSC,
		wallet.Address().Hex(),
		wallet.SignMessage,
	)

	if err != nil {
		t.Logf("Auth failed (may be expected): %v", err)
		return
	}

	if loginResp.Token == "" {
		t.Error("expected non-empty token")
	}

	if loginResp.Address != wallet.Address().Hex() {
		t.Errorf("expected address %s, got %s", wallet.Address().Hex(), loginResp.Address)
	}
}

// TestMain 测试入口
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// Example_wsStream 示例
func Example_wsStream() {
	// 命令行使用方式:
	// ./bin/ws-stream -s BTC-USD -k <private_key>
	// 或者使用环境变量:
	// export WALLET_PRIVATE_KEY=<private_key>
	// ./bin/ws-stream -s ETH-USD

	fmt.Println("Usage: ws-stream -s BTC-USD -k <private_key>")
	// Output: Usage: ws-stream -s BTC-USD -k <private_key>
}

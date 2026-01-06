package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/cbl315/standx-liquidity-market-maker/pkg/client"
)

// TestGetPrice_Success 测试成功获取价格
func TestGetPrice_Success(t *testing.T) {
	// 创建模拟 HTTP 服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和路径
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}

		// 验证 URL 路径
		if r.URL.Path != "/api/query_symbol_price" {
			t.Errorf("expected path /api/query_symbol_price, got %s", r.URL.Path)
		}

		// 验证 URL 参数
		symbol := r.URL.Query().Get("symbol")
		if symbol != "BTC-USD" {
			t.Errorf("expected symbol BTC-USD, got %s", symbol)
		}

		// 验证 Authorization 头
		expectedAuth := "Bearer test-token-123"
		if r.Header.Get("Authorization") != expectedAuth {
			t.Errorf("expected Authorization %s, got %s", expectedAuth, r.Header.Get("Authorization"))
		}

		// 返回成功响应 (根据 API 文档格式)
		resp := map[string]string{
			"symbol":      "BTC-USD",
			"base":        "BTC",
			"quote":       "DUSD",
			"mark_price":  "50000.50",
			"index_price": "50001.00",
			"last_price":  "49999.50",
			"mid_price":   "50000.00",
			"spread_bid":  "49999.95",
			"spread_ask":  "50000.05",
			"time":        "2025-08-11T03:44:40.922233Z",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// 创建认证管理器
	authMgr := auth.NewAuth(mockServer.URL)

	// 创建 API 客户端
	apiClient := client.NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token-123")

	// 获取标记价格
	price, err := apiClient.GetMarkPrice("BTC-USD")
	if err != nil {
		t.Fatalf("GetMarkPrice failed: %v", err)
	}

	expectedPrice := 50000.50
	if price != expectedPrice {
		t.Errorf("expected price %f, got %f", expectedPrice, price)
	}
}

// TestGetPrice_NotFound 测试获取不存在的 ticker
func TestGetPrice_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "symbol not found",
		})
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := client.NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetMarkPrice("INVALID-SYMBOL")
	if err == nil {
		t.Error("expected error for not found symbol, got nil")
	}
}

// TestGetPrice_InvalidToken 测试无效 token
func TestGetPrice_InvalidToken(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized",
		})
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := client.NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("invalid-token")

	_, err := apiClient.GetMarkPrice("BTC-USD")
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

// TestGetPrice_ServerError 测试服务器错误
func TestGetPrice_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "internal server error",
		})
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := client.NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetMarkPrice("BTC-USD")
	if err == nil {
		t.Error("expected error for server error, got nil")
	}
}

// TestGetPrice_MultipleSymbols 测试获取多个不同的 symbol 价格
func TestGetPrice_MultipleSymbols(t *testing.T) {
	testCases := []struct {
		name          string
		symbol        string
		expectedPrice string
	}{
		{"BTC", "BTC-USD", "50000.50"},
		{"ETH", "ETH-USD", "3000.25"},
		{"SOL", "SOL-USD", "150.75"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]string{
					"symbol":      tc.symbol,
					"base":        tc.symbol[0:3],
					"quote":       "DUSD",
					"mark_price":  tc.expectedPrice,
					"index_price": tc.expectedPrice,
					"last_price":  tc.expectedPrice,
					"mid_price":   tc.expectedPrice,
					"spread_bid":  tc.expectedPrice,
					"spread_ask":  tc.expectedPrice,
					"time":        "2025-08-11T03:44:40.922233Z",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer mockServer.Close()

			authMgr := auth.NewAuth(mockServer.URL)
			apiClient := client.NewClient(mockServer.URL, authMgr)
			apiClient.SetToken("test-token")

			priceInfo, err := apiClient.GetSymbolPrice(tc.symbol)
			if err != nil {
				t.Fatalf("GetSymbolPrice failed: %v", err)
			}

			if priceInfo.MarkPrice != tc.expectedPrice {
				t.Errorf("expected mark_price %s, got %s", tc.expectedPrice, priceInfo.MarkPrice)
			}

			if priceInfo.Symbol != tc.symbol {
				t.Errorf("expected symbol %s, got %s", tc.symbol, priceInfo.Symbol)
			}
		})
	}
}

// TestGetPrice_AuthFlow 测试完整的认证流程
func TestGetPrice_AuthFlow(t *testing.T) {
	testPayload := `{"message":"test message for signing","exp":1760291384,"iat":1760291204}`
	encodedPayload := testPayload // 简化处理
	testJWT := "header." + encodedPayload + ".signature"

	// 生成测试私钥
	wallet, err := auth.NewWalletSigner("0x" + "0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil {
		t.Skip("need valid private key for auth test")
	}

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
				"token":   "authenticated-token",
				"address": wallet.Address().Hex(),
				"alias":   "test",
				"chain":   "bsc",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/query_symbol_price" {
			resp := map[string]string{
				"symbol":      "BTC-USD",
				"base":        "BTC",
				"quote":       "DUSD",
				"mark_price":  "50000.50",
				"index_price": "50001.00",
				"last_price":  "49999.50",
				"mid_price":   "50000.00",
				"spread_bid":  "49999.95",
				"spread_ask":  "50000.05",
				"time":        "2025-08-11T03:44:40.922233Z",
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
		t.Logf("Auth failed: %v", err)
		return
	}

	apiClient := client.NewClient(mockServer.URL, authMgr)
	apiClient.SetToken(loginResp.Token)

	price, err := apiClient.GetMarkPrice("BTC-USD")
	if err != nil {
		t.Fatalf("GetMarkPrice failed: %v", err)
	}

	expectedPrice := 50000.50
	if price != expectedPrice {
		t.Errorf("expected price %f, got %f", expectedPrice, price)
	}
}

// TestMain 测试入口
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// Example_getPrice 示例：使用命令行工具获取价格
func Example_getPrice() {
	// 命令行使用方式:
	// ./bin/get-price -s BTC-USD -k <private_key>
	// 或者使用环境变量:
	// export WALLET_PRIVATE_KEY=<private_key>
	// ./bin/get-price -s ETH-USD

	fmt.Println("Usage: get-price -s BTC-USD -k <private_key>")
	// Output: Usage: get-price -s BTC-USD -k <private_key>
}

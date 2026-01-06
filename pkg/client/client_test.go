package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
)

// TestGetOpenOrders_Success 测试成功获取挂单
func TestGetOpenOrders_Success(t *testing.T) {
	// 期望的订单列表 (根据 API 文档格式)
	expectedOrders := []Order{
		{
			ID:           1820682,
			Symbol:       "BTC-USD",
			Side:         OrderBid,
			OrderType:    OrderTypeLimit,
			Price:        "50000.00",
			Qty:          "0.060",
			FillQty:      "0",
			FillAvgPrice: "0",
			Status:       "new",
			TimeInForce:  TimeInForceGTC,
			ReduceOnly:   false,
			Leverage:     "10",
			Margin:       "0",
			CreatedAt:    "2025-08-11T03:35:25.559151Z",
			UpdatedAt:    "2025-08-11T03:35:25.559151Z",
			ClOrdID:      "01K2BK4ZKQE0C308SRD39P8N9Z",
			PositionID:   15,
			AvailLocked:  "3.071880000",
		},
		{
			ID:           1820683,
			Symbol:       "ETH-USD",
			Side:         OrderAsk,
			OrderType:    OrderTypeLimit,
			Price:        "3000.00",
			Qty:          "1.000",
			FillQty:      "0.500",
			FillAvgPrice: "2995.00",
			Status:       "new",
			TimeInForce:  TimeInForceGTC,
			ReduceOnly:   false,
			Leverage:     "10",
			Margin:       "0",
			CreatedAt:    "2025-08-11T03:35:25.559151Z",
			UpdatedAt:    "2025-08-11T03:35:25.559151Z",
			ClOrdID:      "01K2BK4ZKQE0C308SRD39P8N9Z",
			PositionID:   16,
			AvailLocked:  "0.000000000",
		},
	}

	// 创建模拟 HTTP 服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和路径
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}

		if r.URL.Path != "/api/query_open_orders" {
			t.Errorf("expected path /api/query_open_orders, got %s", r.URL.Path)
		}

		// 验证 Authorization 头
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}

		// 返回成功响应 (根据 API 文档格式)
		resp := OpenOrdersResponse{
			PageSize: 2,
			Result:   expectedOrders,
			Total:    2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	// 创建客户端
	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	// 调用 GetOpenOrders
	orders, err := apiClient.GetOpenOrders()
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	// 验证结果
	if len(orders) != len(expectedOrders) {
		t.Errorf("expected %d orders, got %d", len(expectedOrders), len(orders))
	}

	for i, order := range orders {
		expected := expectedOrders[i]
		if order.ID != expected.ID {
			t.Errorf("order %d: expected ID %d, got %d", i, expected.ID, order.ID)
		}
		if order.Symbol != expected.Symbol {
			t.Errorf("order %d: expected Symbol %s, got %s", i, expected.Symbol, order.Symbol)
		}
		if order.Side != expected.Side {
			t.Errorf("order %d: expected Side %s, got %s", i, expected.Side, order.Side)
		}
		if order.Price != expected.Price {
			t.Errorf("order %d: expected Price %s, got %s", i, expected.Price, order.Price)
		}
		if order.Status != expected.Status {
			t.Errorf("order %d: expected Status %s, got %s", i, expected.Status, order.Status)
		}
	}
}

// TestGetOpenOrders_Empty 测试获取空订单列表
func TestGetOpenOrders_Empty(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回空订单列表 (根据 API 文档格式)
		resp := OpenOrdersResponse{
			PageSize: 0,
			Result:   []Order{},
			Total:    0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	orders, err := apiClient.GetOpenOrders()
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	if len(orders) != 0 {
		t.Errorf("expected 0 orders, got %d", len(orders))
	}
}

// TestGetOpenOrders_WithSymbol 测试带 symbol 参数
func TestGetOpenOrders_WithSymbol(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 symbol 参数
		symbol := r.URL.Query().Get("symbol")
		if symbol != "BTC-USD" {
			t.Errorf("expected symbol BTC-USD, got %s", symbol)
		}

		resp := OpenOrdersResponse{
			PageSize: 1,
			Result: []Order{
				{
					ID:      1,
					Symbol:  "BTC-USD",
					Side:    OrderBid,
					Price:   "50000.00",
					Qty:     "0.060",
					FillQty: "0",
					Status:  "new",
				},
			},
			Total: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	// 使用 symbol 参数调用
	orders, err := apiClient.GetOpenOrders("BTC-USD")
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	if len(orders) != 1 {
		t.Errorf("expected 1 order, got %d", len(orders))
	}

	if orders[0].Symbol != "BTC-USD" {
		t.Errorf("expected symbol BTC-USD, got %s", orders[0].Symbol)
	}
}

// TestGetOpenOrders_HTTPError 测试 HTTP 错误响应
func TestGetOpenOrders_HTTPError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request",
		})
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetOpenOrders()
	if err == nil {
		t.Error("expected error for bad request, got nil")
	}
}

// TestGetOpenOrders_NetworkError 测试网络错误
func TestGetOpenOrders_NetworkError(t *testing.T) {
	authMgr := auth.NewAuth("http://invalid-host-that-does-not-exist-12345.com")
	apiClient := NewClient("http://invalid-host-that-does-not-exist-12345.com", authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetOpenOrders()
	if err == nil {
		t.Error("expected error for network failure, got nil")
	}
}

// TestGetOpenOrders_SignatureHeaders 测试签名头 (GET 请求不需要签名)
func TestGetOpenOrders_SignatureHeaders(t *testing.T) {
	var capturedHeaders http.Header

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()

		resp := OpenOrdersResponse{
			PageSize: 0,
			Result:   []Order{},
			Total:    0,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetOpenOrders()
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	// 验证不需要签名头 (GET 请求)
	if capturedHeaders.Get("x-request-sign-version") != "" {
		t.Error("GET request should not have x-request-sign-version header")
	}

	if capturedHeaders.Get("x-request-id") != "" {
		t.Error("GET request should not have x-request-id header")
	}

	// 验证 Authorization 头
	expectedAuth := "Bearer test-token"
	if capturedHeaders.Get("Authorization") != expectedAuth {
		t.Errorf("expected Authorization %s, got %s", expectedAuth, capturedHeaders.Get("Authorization"))
	}
}

// TestGetOpenOrders_ServerError 测试服务器错误
func TestGetOpenOrders_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "internal server error",
		})
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	_, err := apiClient.GetOpenOrders()
	if err == nil {
		t.Error("expected error for server error, got nil")
	}
}

// TestGetOpenOrders_MultipleOrders 测试获取多个订单
func TestGetOpenOrders_MultipleOrders(t *testing.T) {
	// 创建 10 个测试订单
	expectedOrders := make([]Order, 10)
	for i := 0; i < 10; i++ {
		expectedOrders[i] = Order{
			ID:      1000 + i,
			Symbol:  "BTC-USD",
			Side:    OrderBid,
			Price:   "50000.00",
			Qty:     "0.060",
			FillQty: "0",
			Status:  "new",
		}
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OpenOrdersResponse{
			PageSize: 10,
			Result:   expectedOrders,
			Total:    10,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	authMgr := auth.NewAuth(mockServer.URL)
	apiClient := NewClient(mockServer.URL, authMgr)
	apiClient.SetToken("test-token")

	orders, err := apiClient.GetOpenOrders()
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}

	if len(orders) != 10 {
		t.Errorf("expected 10 orders, got %d", len(orders))
	}

	// 验证每个订单的 ID 唯一性
	orderIDs := make(map[int]bool)
	for _, order := range orders {
		if orderIDs[order.ID] {
			t.Errorf("duplicate order ID found: %d", order.ID)
		}
		orderIDs[order.ID] = true
	}
}

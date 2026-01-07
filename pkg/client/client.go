package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cbl315/standx-liquidity-market-maker/pkg/auth"
	"github.com/google/uuid"
)

// Client API 客户端
type Client struct {
	httpClient *http.Client
	auth       *auth.Auth
	token      string
	baseURL    string
}

// NewClient 创建 API 客户端
func NewClient(baseURL string, authMgr *auth.Auth) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		auth:    authMgr,
		baseURL: baseURL,
	}
}

// SetToken 设置访问令牌
func (c *Client) SetToken(token string) {
	c.token = token
}

// NewOrder 下单
func (c *Client) NewOrder(req *NewOrderRequest) (*OrderResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	reqID := uuid.New().String()
	timestamp := time.Now().UnixMilli()

	signatures := c.auth.SignRequest(payload, reqID, timestamp)

	resp, err := c.doRequest("POST", "/api/new_order", payload, signatures)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("new order failed: %s, %s", string(body), payload)
	}

	var result OrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// 检查响应中的 code 字段
	if result.Code != 0 {
		return nil, fmt.Errorf("new order failed: code=%d, message=%s, payload=%s", result.Code, result.Message, string(payload))
	}

	return &result, nil
}

// CancelOrder 取消订单（使用 client order ID）
func (c *Client) CancelOrder(clOrdID string) error {
	reqBody := map[string]string{
		"cl_ord_id": clOrdID,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	reqID := uuid.New().String()
	timestamp := time.Now().UnixMilli()

	signatures := c.auth.SignRequest(payload, reqID, timestamp)

	resp, err := c.doRequest("POST", "/api/cancel_order", payload, signatures)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel order failed: %s", string(body))
	}

	return nil
}

// CancelAllOrders 取消所有订单
func (c *Client) CancelAllOrders() error {
	reqBody := map[string]string{}
	payload, _ := json.Marshal(reqBody)

	reqID := uuid.New().String()
	timestamp := time.Now().UnixMilli()

	signatures := c.auth.SignRequest(payload, reqID, timestamp)

	resp, err := c.doRequest("POST", "/api/cancel_all_orders", payload, signatures)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel all orders failed: %s", string(body))
	}

	return nil
}

// GetOpenOrders 获取当前挂单
func (c *Client) GetOpenOrders(symbol ...string) ([]Order, error) {
	// 构建 URL
	url := c.baseURL + "/api/query_open_orders"
	if len(symbol) > 0 && symbol[0] != "" {
		url += "?symbol=" + symbol[0]
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get open orders failed: %s", string(body))
	}

	var result OpenOrdersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// GetOpenOrdersByStatus 按状态获取订单
func (c *Client) GetOpenOrdersByStatus(symbol, status string) ([]Order, error) {
	url := fmt.Sprintf("%s/api/query_orders?symbol=%s&status=%s", c.baseURL, symbol, status)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get open orders by status failed: %s", string(body))
	}

	var result OpenOrdersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

// GetPosition 获取仓位
func (c *Client) GetPosition(symbol string) (*Position, error) {
	url := fmt.Sprintf("%s/api/query_positions?symbol=%s", c.baseURL, symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get position failed: %s", string(body))
	}

	var result []Position
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	return &result[0], nil
}

// GetSymbolPrice 获取 Symbol 价格
func (c *Client) GetSymbolPrice(symbol string) (*SymbolPriceInfo, error) {
	url := fmt.Sprintf("%s/api/query_symbol_price?symbol=%s", c.baseURL, symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get symbol price failed: %s", string(body))
	}

	var result SymbolPriceInfo
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetMarkPrice 获取标记价格 (保留向后兼容)
func (c *Client) GetMarkPrice(symbol string) (float64, error) {
	priceInfo, err := c.GetSymbolPrice(symbol)
	if err != nil {
		return 0, err
	}

	// 解析价格字符串为 float64
	return strconv.ParseFloat(priceInfo.MarkPrice, 64)
}

// BalanceResponse 账户余额响应
type BalanceResponse struct {
	IsolatedBalance string `json:"isolated_balance"`
	IsolatedUPNL    string `json:"isolated_upnl"`
	CrossBalance    string `json:"cross_balance"`
	CrossMargin     string `json:"cross_margin"`
	CrossUPNL       string `json:"cross_upnl"`
	Locked          string `json:"locked"`
	CrossAvailable  string `json:"cross_available"`
	Balance         string `json:"balance"`
	UPNL            string `json:"upnl"`
	Equity          string `json:"equity"`
	PNLFreeze       string `json:"pnl_freeze"`
}

// GetBalance 获取账户余额
func (c *Client) GetBalance() (*BalanceResponse, error) {
	url := c.baseURL + "/api/query_balance"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get balance failed: %s", string(body))
	}

	var result BalanceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// doRequest 执行 HTTP 请求
func (c *Client) doRequest(method, path string, body []byte, signatures map[string]string) (*http.Response, error) {
	url := c.baseURL + path

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	// 添加签名头
	for k, v := range signatures {
		req.Header.Set(k, v)
	}

	return c.httpClient.Do(req)
}

// FormatPrice 格式化价格（保留两位小数）
func FormatPrice(price float64) string {
	return strconv.FormatFloat(price, 'f', 2, 64)
}

// FormatQty 格式化数量
func FormatQty(qty float64) string {
	return strconv.FormatFloat(qty, 'f', -1, 64)
}

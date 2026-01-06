package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
)

// TestNewAuth 测试创建认证实例
func TestNewAuth(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	if auth == nil {
		t.Fatal("NewAuth returned nil")
	}

	if auth.baseURL != "https://api.standx.com" {
		t.Errorf("expected baseURL https://api.standx.com, got %s", auth.baseURL)
	}

	if auth.ed25519PrivateKey == nil {
		t.Error("private key should not be nil")
	}

	if auth.ed25519PublicKey == nil {
		t.Error("public key should not be nil")
	}

	if auth.requestID == "" {
		t.Error("requestID should not be empty")
	}

	// 验证 requestId 是有效的 base58
	_, err := base58.Decode(auth.requestID)
	if err != nil {
		t.Errorf("requestID should be valid base58: %v", err)
	}

	// 验证 requestId 与公钥匹配
	decodedRequestID, _ := base58.Decode(auth.requestID)
	if !strings.EqualFold(string(auth.ed25519PublicKey), string(decodedRequestID)) {
		t.Error("requestID should match the public key")
	}
}

// TestGetRequestID 测试获取 requestId
func TestGetRequestID(t *testing.T) {
	auth := NewAuth("https://api.standx.com")
	requestID := auth.GetRequestID()

	if requestID == "" {
		t.Error("GetRequestID returned empty string")
	}

	if requestID != auth.requestID {
		t.Errorf("expected %s, got %s", auth.requestID, requestID)
	}
}

// TestSignRequest 测试签名请求
func TestSignRequest(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	payload := []byte(`{"symbol":"BTC-USD","side":"buy","qty":"1.0"}`)
	reqID := "test-request-id-123"
	timestamp := int64(1234567890000)

	signatures := auth.SignRequest(payload, reqID, timestamp)

	// 验证返回的签名头
	if signatures["x-request-sign-version"] != "v1" {
		t.Errorf("expected version v1, got %s", signatures["x-request-sign-version"])
	}

	if signatures["x-request-id"] != reqID {
		t.Errorf("expected request_id %s, got %s", reqID, signatures["x-request-id"])
	}

	if signatures["x-request-timestamp"] != "1234567890000" {
		t.Errorf("expected timestamp 1234567890000, got %s", signatures["x-request-timestamp"])
	}

	if signatures["x-request-signature"] == "" {
		t.Error("signature should not be empty")
	}

	// 验证签名是有效的 base64
	_, err := base64.StdEncoding.DecodeString(signatures["x-request-signature"])
	if err != nil {
		t.Errorf("signature should be valid base64: %v", err)
	}

	// 验证签名长度 (ed25519 签名是 64 字节)
	sigBytes, _ := base64.StdEncoding.DecodeString(signatures["x-request-signature"])
	if len(sigBytes) != 64 {
		t.Errorf("ed25519 signature should be 64 bytes, got %d", len(sigBytes))
	}
}

// TestSignRequest_VerifySignature 测试签名验证
func TestSignRequest_VerifySignature(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	payload := []byte(`{"test":"data"}`)
	reqID := "verify-test-id"
	timestamp := int64(1704067200000)

	signatures := auth.SignRequest(payload, reqID, timestamp)

	// 重建消息
	version := "v1"
	message := version + "," + reqID + "," + "1704067200000" + "," + string(payload)

	// 解码签名
	sigBytes, err := base64.StdEncoding.DecodeString(signatures["x-request-signature"])
	if err != nil {
		t.Fatalf("failed to decode signature: %v", err)
	}

	// 验证签名
	if !ed25519.Verify(auth.ed25519PublicKey, []byte(message), sigBytes) {
		t.Error("signature verification failed")
	}
}

// TestParseJWT 测试解析 JWT
func TestParseJWT(t *testing.T) {
	// 创建一个测试 JWT (header.payload.signature)
	// payload: {"domain":"standx.com","message":"test message"}
	testPayload := base64.StdEncoding.EncodeToString([]byte(`{"domain":"standx.com","uri":"https://standx.com","statement":"Sign in","version":"1","chainId":56,"nonce":"test123","address":"0xtest","requestId":"req123","issuedAt":"2024-01-01T00:00:00Z","message":"standx.com wants you to sign in","exp":1760291384,"iat":1760291204}`))
	testJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." + testPayload + ".signature"

	data, err := parseJWT(testJWT)
	if err != nil {
		t.Fatalf("parseJWT failed: %v", err)
	}

	if data.Domain != "standx.com" {
		t.Errorf("expected domain standx.com, got %s", data.Domain)
	}

	if data.Message == "" {
		t.Error("message should not be empty")
	}

	if data.ChainID != 56 {
		t.Errorf("expected chainId 56, got %d", data.ChainID)
	}
}

// TestParseJWT_InvalidFormat 测试无效 JWT 格式
func TestParseJWT_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		jwt  string
	}{
		{"empty", ""},
		{"no dots", "invalidjwt"},
		{"only one dot", "header.payload"},
		{"three dots", "header.payload.extra.signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseJWT(tt.jwt)
			if err == nil {
				t.Error("expected error for invalid JWT, got nil")
			}
		})
	}
}

// TestParseJWT_Base64Padding 测试 Base64 padding 处理
func TestParseJWT_Base64Padding(t *testing.T) {
	// 测试需要添加 padding 的情况
	// 标准 base64 编码的 payload 长度应该是 4 的倍数
	testPayload := []byte(`{"test":"value"}`)
	// 编码后可能需要 padding
	encoded := base64.StdEncoding.EncodeToString(testPayload)

	// 构建 JWT
	testJWT := "header." + encoded + ".signature"

	data, err := parseJWT(testJWT)
	if err != nil {
		t.Fatalf("parseJWT with padding failed: %v", err)
	}

	if data == nil {
		t.Error("parsed data should not be nil")
	}
}

// TestGetToken 测试获取 token
func TestGetToken(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	// 初始状态 token 为空
	if auth.GetToken() != "" {
		t.Error("initial token should be empty")
	}

	// 设置 token
	auth.token = "test-token-123"

	if auth.GetToken() != "test-token-123" {
		t.Errorf("expected test-token-123, got %s", auth.GetToken())
	}
}

// TestPrepareSignInRequest_Marshal 测试请求序列化
func TestPrepareSignInRequest_Marshal(t *testing.T) {
	req := PrepareSignInRequest{
		Address:   "0x1234567890abcdef",
		RequestID: "test-request-id",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if !strings.Contains(string(data), "0x1234567890abcdef") {
		t.Error("marshaled data should contain address")
	}

	if !strings.Contains(string(data), "test-request-id") {
		t.Error("marshaled data should contain requestID")
	}
}

// TestSignRequest_DifferentInputs 测试不同输入产生不同签名
func TestSignRequest_DifferentInputs(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	payload1 := []byte(`{"test":"data1"}`)
	payload2 := []byte(`{"test":"data2"}`)
	reqID := "test-id"
	timestamp := int64(1704067200000)

	sig1 := auth.SignRequest(payload1, reqID, timestamp)
	sig2 := auth.SignRequest(payload2, reqID, timestamp)

	if sig1["x-request-signature"] == sig2["x-request-signature"] {
		t.Error("different payloads should produce different signatures")
	}
}

// TestSignRequest_SameInput_SameSignature 测试相同输入产生相同签名
func TestSignRequest_SameInput_SameSignature(t *testing.T) {
	auth := NewAuth("https://api.standx.com")

	payload := []byte(`{"test":"data"}`)
	reqID := "test-id"
	timestamp := int64(1704067200000)

	sig1 := auth.SignRequest(payload, reqID, timestamp)
	sig2 := auth.SignRequest(payload, reqID, timestamp)

	if sig1["x-request-signature"] != sig2["x-request-signature"] {
		t.Error("same inputs should produce same signatures")
	}
}

// TestPrepareSignIn_Success 测试准备签名成功
func TestPrepareSignIn_Success(t *testing.T) {
	// 创建模拟 HTTP 服务器
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		// 验证 URL 路径
		if !strings.Contains(r.URL.Path, "prepare-signin") {
			t.Errorf("expected prepare-signin in path, got %s", r.URL.Path)
		}

		// 验证请求体
		var req PrepareSignInRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}

		if req.RequestID == "" {
			t.Error("requestID should not be empty")
		}

		// 返回成功响应
		resp := PrepareSignInResponse{
			Success:    true,
			SignedData: "header.payload.signature",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	signedData, err := auth.prepareSignIn(ChainBSC, "0x1234567890abcdef")
	if err != nil {
		t.Fatalf("prepareSignIn failed: %v", err)
	}

	if signedData != "header.payload.signature" {
		t.Errorf("expected signedData 'header.payload.signature', got %s", signedData)
	}
}

// TestPrepareSignIn_Failure 测试准备签名失败
func TestPrepareSignIn_Failure(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回失败响应
		resp := PrepareSignInResponse{
			Success:    false,
			SignedData: "",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	_, err := auth.prepareSignIn(ChainBSC, "0x1234567890abcdef")
	if err == nil {
		t.Error("expected error for failed prepare sign in, got nil")
	}
}

// TestPrepareSignIn_NetworkError 测试网络错误
func TestPrepareSignIn_NetworkError(t *testing.T) {
	auth := NewAuth("http://invalid-host-that-does-not-exist-12345.com")

	_, err := auth.prepareSignIn(ChainBSC, "0x1234567890abcdef")
	if err == nil {
		t.Error("expected error for network failure, got nil")
	}
}

// TestLogin_Success 测试登录成功
func TestLogin_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		// 验证 URL 路径
		if !strings.Contains(r.URL.Path, "login") {
			t.Errorf("expected login in path, got %s", r.URL.Path)
		}

		// 验证请求体
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}

		if req.Signature == "" {
			t.Error("signature should not be empty")
		}

		if req.SignedData == "" {
			t.Error("signedData should not be empty")
		}

		if req.ExpiresSeconds != 604800 {
			t.Errorf("expected ExpiresSeconds 604800, got %d", req.ExpiresSeconds)
		}

		// 返回成功响应
		resp := LoginResponse{
			Token:      "test-access-token-123",
			Address:    "0x1234567890abcdef",
			Alias:      "test-user",
			Chain:      "bsc",
			PerpsAlpha: true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	loginResp, err := auth.login(ChainBSC, "0xabc123", "header.payload.signature")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if loginResp.Token != "test-access-token-123" {
		t.Errorf("expected token 'test-access-token-123', got %s", loginResp.Token)
	}

	if loginResp.Address != "0x1234567890abcdef" {
		t.Errorf("expected address '0x1234567890abcdef', got %s", loginResp.Address)
	}

	if !loginResp.PerpsAlpha {
		t.Error("expected PerpsAlpha to be true")
	}
}

// TestLogin_NetworkError 测试登录网络错误
func TestLogin_NetworkError(t *testing.T) {
	auth := NewAuth("http://invalid-host-that-does-not-exist-12345.com")

	_, err := auth.login(ChainBSC, "0xabc123", "header.payload.signature")
	if err == nil {
		t.Error("expected error for network failure, got nil")
	}
}

// TestAuthenticate_Success 测试完整认证流程
func TestAuthenticate_Success(t *testing.T) {
	// 准备一个有效的 JWT
	testPayload := `{"domain":"standx.com","uri":"https://standx.com","statement":"Sign in","version":"1","chainId":56,"nonce":"test123","address":"0x1234567890abcdef","requestId":"req123","issuedAt":"2024-01-01T00:00:00Z","message":"test message for signing","exp":1760291384,"iat":1760291204}`
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(testPayload))
	testJWT := "header." + encodedPayload + ".signature"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "prepare-signin") {
			resp := PrepareSignInResponse{
				Success:    true,
				SignedData: testJWT,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if strings.Contains(r.URL.Path, "login") {
			resp := LoginResponse{
				Token:      "authenticated-token-456",
				Address:    "0x1234567890abcdef",
				Alias:      "test-user",
				Chain:      "bsc",
				PerpsAlpha: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	// 模拟签名函数
	signFn := func(message string) (string, error) {
		return "0xabcdef123456", nil
	}

	loginResp, err := auth.Authenticate(ChainBSC, "0x1234567890abcdef", signFn)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	if loginResp.Token != "authenticated-token-456" {
		t.Errorf("expected token 'authenticated-token-456', got %s", loginResp.Token)
	}

	// 验证 token 已保存
	if auth.GetToken() != "authenticated-token-456" {
		t.Errorf("expected token 'authenticated-token-456' to be saved, got %s", auth.GetToken())
	}
}

// TestAuthenticate_PrepareSignInError 测试准备签名阶段错误
func TestAuthenticate_PrepareSignInError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "prepare-signin") {
			resp := PrepareSignInResponse{
				Success:    false,
				SignedData: "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	signFn := func(message string) (string, error) {
		return "0xabc123", nil
	}

	_, err := auth.Authenticate(ChainBSC, "0x1234567890abcdef", signFn)
	if err == nil {
		t.Error("expected error for failed prepare sign in, got nil")
	}
}

// TestAuthenticate_SignError 测试签名函数错误
func TestAuthenticate_SignError(t *testing.T) {
	testPayload := `{"message":"test message","exp":1760291384,"iat":1760291204}`
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(testPayload))
	testJWT := "header." + encodedPayload + ".signature"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "prepare-signin") {
			resp := PrepareSignInResponse{
				Success:    true,
				SignedData: testJWT,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	// 返回错误的签名函数
	signFn := func(message string) (string, error) {
		return "", fmt.Errorf("signing failed")
	}

	_, err := auth.Authenticate(ChainBSC, "0x1234567890abcdef", signFn)
	if err == nil {
		t.Error("expected error for failed sign, got nil")
	}
}

// TestAuthenticate_LoginError 测试登录阶段错误
func TestAuthenticate_LoginError(t *testing.T) {
	testPayload := `{"message":"test message","exp":1760291384,"iat":1760291204}`
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(testPayload))
	testJWT := "header." + encodedPayload + ".signature"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "prepare-signin") {
			resp := PrepareSignInResponse{
				Success:    true,
				SignedData: testJWT,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if strings.Contains(r.URL.Path, "login") {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	auth := NewAuth(mockServer.URL)

	signFn := func(message string) (string, error) {
		return "0xabc123", nil
	}

	_, err := auth.Authenticate(ChainBSC, "0x1234567890abcdef", signFn)
	if err == nil {
		t.Error("expected error for failed login, got nil")
	}
}

// TestChainConstants 测试链常量
func TestChainConstants(t *testing.T) {
	if ChainBSC != "bsc" {
		t.Errorf("expected ChainBSC to be 'bsc', got %s", ChainBSC)
	}

	if ChainSolana != "solana" {
		t.Errorf("expected ChainSolana to be 'solana', got %s", ChainSolana)
	}
}

// TestLoginRequest_ExpiresSeconds 测试登录请求过期时间
func TestLoginRequest_ExpiresSeconds(t *testing.T) {
	req := LoginRequest{
		Signature:      "0xabc123",
		SignedData:     "header.payload.signature",
		ExpiresSeconds: 604800, // 7 天
	}

	if req.ExpiresSeconds != 604800 {
		t.Errorf("expected ExpiresSeconds 604800, got %d", req.ExpiresSeconds)
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if !strings.Contains(string(data), "604800") {
		t.Error("marshaled data should contain expiresSeconds")
	}
}

package auth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mr-tron/base58"
)

// Auth 认证管理器
type Auth struct {
	ed25519PrivateKey ed25519.PrivateKey
	ed25519PublicKey  ed25519.PublicKey
	requestID         string
	baseURL           string
	token             string
	httpClient        *http.Client
}

// NewAuth 创建认证实例
func NewAuth(baseURL string) *Auth {
	// 生成 ed25519 密钥对
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(fmt.Sprintf("failed to generate ed25519 key pair: %v", err))
	}

	// requestId 是 base58 编码的公钥
	requestID := base58.Encode(publicKey)

	return &Auth{
		ed25519PrivateKey: privateKey,
		ed25519PublicKey:  publicKey,
		requestID:         requestID,
		baseURL:           baseURL,
		httpClient:        &http.Client{},
	}
}

// GetRequestID 获取 requestId
func (a *Auth) GetRequestID() string {
	return a.requestID
}

// GetToken 获取当前 token
func (a *Auth) GetToken() string {
	return a.token
}

// Authenticate 执行认证流程
func (a *Auth) Authenticate(chain Chain, address string, signFn SignFunc) (*LoginResponse, error) {
	// 1. 准备签名 - 获取 signedData
	signedData, err := a.prepareSignIn(chain, address)
	if err != nil {
		return nil, fmt.Errorf("prepare sign in failed: %w", err)
	}

	// 2. 解析 signedData JWT payload
	payload, err := parseJWT(signedData)
	if err != nil {
		return nil, fmt.Errorf("parse JWT failed: %w", err)
	}

	// 3. 使用钱包私钥签名 message
	signature, err := signFn(payload.Message)
	if err != nil {
		return nil, fmt.Errorf("sign message failed: %w", err)
	}

	// 4. 登录获取 access token
	loginResp, err := a.login(chain, signature, signedData)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	// 保存 token
	a.token = loginResp.Token

	return loginResp, nil
}

// prepareSignIn 准备签名
func (a *Auth) prepareSignIn(chain Chain, address string) (string, error) {
	url := fmt.Sprintf("%s/v1/offchain/prepare-signin?chain=%s", a.baseURL, chain)

	reqBody := PrepareSignInRequest{
		Address:   address,
		RequestID: a.requestID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := a.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result PrepareSignInResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("prepare sign in failed: %s", string(respBody))
	}

	return result.SignedData, nil
}

// login 登录获取 token
func (a *Auth) login(chain Chain, signature, signedData string) (*LoginResponse, error) {
	url := fmt.Sprintf("%s/v1/offchain/login?chain=%s", a.baseURL, chain)

	reqBody := LoginRequest{
		Signature:      signature,
		SignedData:     signedData,
		ExpiresSeconds: 604800, // 7 天
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return nil, err
	}

	return &loginResp, nil
}

// SignRequest 签名请求 (Body Signature Flow)
func (a *Auth) SignRequest(payload []byte, reqID string, timestamp int64) map[string]string {
	version := "v1"
	// 构建消息: {version},{id},{timestamp},{payload}
	message := fmt.Sprintf("%s,%s,%d,%s", version, reqID, timestamp, string(payload))

	// ed25519 签名
	signature := ed25519.Sign(a.ed25519PrivateKey, []byte(message))

	// Base64 编码
	signatureB64 := base64.StdEncoding.EncodeToString(signature)

	return map[string]string{
		"x-request-sign-version": version,
		"x-request-id":          reqID,
		"x-request-timestamp":   fmt.Sprintf("%d", timestamp),
		"x-request-signature":   signatureB64,
	}
}

// parseJWT 简单解析 JWT payload (不验证签名)
func parseJWT(token string) (*SignedData, error) {
	parts := bytes.Split([]byte(token), []byte("."))
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// 解码 payload
	payload := parts[1]
	// 添加 padding
	if len(payload)%4 != 0 {
		padding := 4 - (len(payload) % 4)
		payload = append(payload, bytes.Repeat([]byte("="), padding)...)
	}

	decoded, err := base64.StdEncoding.DecodeString(string(payload))
	if err != nil {
		return nil, err
	}

	var data SignedData
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

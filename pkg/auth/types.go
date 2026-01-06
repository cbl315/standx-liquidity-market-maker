package auth

// Chain 区块链类型
type Chain string

const (
	ChainBSC    Chain = "bsc"
	ChainSolana Chain = "solana"
)

// SignedData 认证签名数据
type SignedData struct {
	Domain    string `json:"domain"`
	URI       string `json:"uri"`
	Statement string `json:"statement"`
	Version   string `json:"version"`
	ChainID   int    `json:"chainId"`
	Nonce     string `json:"nonce"`
	Address   string `json:"address"`
	RequestID string `json:"requestId"`
	IssuedAt  string `json:"issuedAt"`
	Message   string `json:"message"`
	Exp       int64  `json:"exp"`
	Iat       int64  `json:"iat"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token      string `json:"token"`
	Address    string `json:"address"`
	Alias      string `json:"alias"`
	Chain      string `json:"chain"`
	PerpsAlpha bool   `json:"perpsAlpha"`
}

// PrepareSignInRequest 准备签名请求
type PrepareSignInRequest struct {
	Address   string `json:"address"`
	RequestID string `json:"requestId"`
}

// PrepareSignInResponse 准备签名响应
type PrepareSignInResponse struct {
	Success   bool   `json:"success"`
	SignedData string `json:"signedData"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Signature     string `json:"signature"`
	SignedData    string `json:"signedData"`
	ExpiresSeconds int   `json:"expiresSeconds,omitempty"`
}

// SignFunc 签名函数类型
type SignFunc func(message string) (string, error)

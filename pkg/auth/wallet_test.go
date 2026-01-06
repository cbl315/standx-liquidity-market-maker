package auth

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestNewWalletSigner 测试创建钱包签名器
func TestNewWalletSigner(t *testing.T) {
	// 生成一个私钥
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	if signer == nil {
		t.Fatal("signer should not be nil")
	}

	if signer.privateKey == nil {
		t.Error("privateKey should not be nil")
	}

	if signer.address == (common.Address{}) {
		t.Error("address should not be zero")
	}
}

// TestNewWalletSigner_WithPrefix 测试带 0x 前缀的私钥
func TestNewWalletSigner_WithPrefix(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := "0x" + hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	if signer.address == (common.Address{}) {
		t.Error("address should not be zero")
	}
}

// TestNewWalletSigner_InvalidPrivateKey 测试无效私钥
func TestNewWalletSigner_InvalidPrivateKey(t *testing.T) {
	tests := []struct {
		name       string
		privateKey string
	}{
		{"empty", ""},
		{"too short", "abc123"},
		{"invalid hex", "xyz123"},
		{"odd length", "abc123456789abcdef0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWalletSigner(tt.privateKey)
			if err == nil {
				t.Error("expected error for invalid private key, got nil")
			}
		})
	}
}

// TestWalletSigner_Address 测试获取地址
func TestWalletSigner_Address(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	expectedAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	if signer.Address() != expectedAddress {
		t.Errorf("expected address %s, got %s", expectedAddress.Hex(), signer.Address().Hex())
	}
}

// TestWalletSigner_SignMessage 测试签名消息
func TestWalletSigner_SignMessage(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	message := "test message for signing"

	signature, err := signer.SignMessage(message)
	if err != nil {
		t.Fatalf("SignMessage failed: %v", err)
	}

	if signature == "" {
		t.Error("signature should not be empty")
	}

	if !strings.HasPrefix(signature, "0x") {
		t.Error("signature should start with 0x")
	}

	// 签名应该是 65 字节 = 130 hex chars + 0x prefix = 132
	if len(signature) != 132 {
		t.Errorf("expected signature length 132, got %d", len(signature))
	}
}

// TestWalletSigner_SignMessage_DifferentMessages 测试不同消息产生不同签名
func TestWalletSigner_SignMessage_DifferentMessages(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	sig1, _ := signer.SignMessage("message 1")
	sig2, _ := signer.SignMessage("message 2")

	if sig1 == sig2 {
		t.Error("different messages should produce different signatures")
	}
}

// TestWalletSigner_SignMessage_SameMessage 测试相同消息产生相同签名
func TestWalletSigner_SignMessage_SameMessage(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	message := "test message"
	sig1, _ := signer.SignMessage(message)
	sig2, _ := signer.SignMessage(message)

	if sig1 != sig2 {
		t.Error("same message should produce same signature")
	}
}

// TestVerifySignature 测试验证签名
func TestVerifySignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	message := "test message for verification"
	signature, err := signer.SignMessage(message)
	if err != nil {
		t.Fatalf("SignMessage failed: %v", err)
	}

	// crypto.SigToPub 需要 V 值为 0 或 1（不含 +27）
	// 所以在验证时需要将 V 值减去 27
	if len(signature) >= 2 && signature[:2] == "0x" {
		signature = signature[2:]
	}

	sigBytes := common.Hex2Bytes(signature)
	if len(sigBytes) == 65 {
		sigBytes[64] -= 27 // 恢复原始 V 值
	}

	hash := messageHash(message)
	pubkey, err := crypto.SigToPub(hash, sigBytes)
	if err != nil {
		t.Fatalf("SigToPub failed: %v", err)
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubkey)
	if recoveredAddress != signer.Address() {
		t.Errorf("signature verification failed: recovered address %s, expected %s", recoveredAddress.Hex(), signer.Address().Hex())
	}
}

// TestVerifySignature_WrongMessage 测试错误的消息验证失败
func TestVerifySignature_WrongMessage(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	message := "test message"
	signature, _ := signer.SignMessage(message)

	// 使用不同的消息验证应该失败
	wrongMessage := "wrong message"
	if VerifySignature(signer.Address(), wrongMessage, signature) {
		t.Error("signature verification with wrong message should fail")
	}
}

// TestVerifySignature_WrongAddress 测试错误的地址验证失败
func TestVerifySignature_WrongAddress(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	privateKeyBytes := crypto.FromECDSA(privateKey)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)

	signer, err := NewWalletSigner(privateKeyHex)
	if err != nil {
		t.Fatalf("NewWalletSigner failed: %v", err)
	}

	message := "test message"
	signature, _ := signer.SignMessage(message)

	// 生成另一个地址
	wrongAddress := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	if VerifySignature(wrongAddress, message, signature) {
		t.Error("signature verification with wrong address should fail")
	}
}

// TestVerifySignature_InvalidSignature 测试无效签名
func TestVerifySignature_InvalidSignature(t *testing.T) {
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	message := "test message"

	tests := []struct {
		name      string
		signature string
	}{
		{"empty", ""},
		{"too short", "0xabc123"},
		{"invalid hex", "0xxyz"},
		{"no prefix", "abcdef1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifySignature(address, message, tt.signature) {
				t.Error("invalid signature should fail verification")
			}
		})
	}
}

// TestMessageHash 测试消息哈希
func TestMessageHash(t *testing.T) {
	message := "test message"
	hash := messageHash(message)

	if len(hash) != 32 {
		t.Errorf("expected hash length 32, got %d", len(hash))
	}

	// 相同消息应该产生相同哈希
	hash2 := messageHash(message)
	if string(hash) != string(hash2) {
		t.Error("same message should produce same hash")
	}

	// 不同消息应该产生不同哈希
	hash3 := messageHash("different message")
	if string(hash) == string(hash3) {
		t.Error("different messages should produce different hashes")
	}
}

// TestHexCharValue 测试十六进制字符值转换
func TestHexCharValue(t *testing.T) {
	tests := []struct {
		input byte
		want  byte
	}{
		{'0', 0}, {'1', 1}, {'2', 2}, {'3', 3}, {'4', 4},
		{'5', 5}, {'6', 6}, {'7', 7}, {'8', 8}, {'9', 9},
		{'a', 10}, {'b', 11}, {'c', 12}, {'d', 13}, {'e', 14}, {'f', 15},
		{'A', 10}, {'B', 11}, {'C', 12}, {'D', 13}, {'E', 14}, {'F', 15},
		{'x', 16}, {'z', 16}, {'!', 16},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := hexCharValue(tt.input)
			if got != tt.want {
				t.Errorf("hexCharValue(%c) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestHexToSignature 测试十六进制转签名
func TestHexToSignature(t *testing.T) {
	// 65 字节签名的十六进制表示 (130 个十六进制字符)
	valid65ByteHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01"

	tests := []struct {
		name    string
		hexStr  string
		wantLen int
		wantErr bool
	}{
		{"valid 65 bytes", valid65ByteHex, 65, false},
		{"odd length", "abc", 0, true},
		{"invalid hex", "xyz123", 0, true},
		{"empty", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hexToSignature(tt.hexStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("hexToSignature() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("hexToSignature() length = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

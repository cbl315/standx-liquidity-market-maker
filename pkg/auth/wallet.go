package auth

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

// WalletSigner 钱包签名器
type WalletSigner struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

// NewWalletSigner 创建钱包签名器
func NewWalletSigner(privateKeyHex string) (*WalletSigner, error) {
	// 移除 0x 前缀
	if len(privateKeyHex) >= 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid public key type")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	return &WalletSigner{
		privateKey: privateKey,
		address:    address,
	}, nil
}

// Address 获取钱包地址
func (w *WalletSigner) Address() common.Address {
	return w.address
}

// SignMessage 签名消息 (EIP-191)
func (w *WalletSigner) SignMessage(message string) (string, error) {
	hash := messageHash(message)
	signature, err := crypto.Sign(hash, w.privateKey)
	if err != nil {
		return "", err
	}

	// 调整 v 值
	if len(signature) != 65 {
		return "", fmt.Errorf("invalid signature length")
	}

	// v = 27 + recovery_id
	signature[64] += 27

	// 使用 hex 编码确保固定长度
	return "0x" + common.Bytes2Hex(signature), nil
}

// messageHash 计算 EIP-191 消息哈希
func messageHash(message string) []byte {
	// Ethereum signed message: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	fullMessage := prefix + message

	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(fullMessage))
	return hash.Sum(nil)
}

// VerifySignature 验证签名
func VerifySignature(address common.Address, message, signatureHex string) bool {
	if len(signatureHex) >= 2 && signatureHex[:2] == "0x" {
		signatureHex = signatureHex[2:]
	}

	// 签名应该是 65 字节 (130 hex chars)
	if len(signatureHex) != 130 {
		return false
	}

	// 使用 common.Hex2Bytes 转换
	signature := common.Hex2Bytes(signatureHex)

	// 调整 V 值: SignMessage 添加了 27，需要减去
	if len(signature) == 65 && signature[64] >= 27 {
		signature[64] -= 27
	}

	hash := messageHash(message)
	pubkey, err := crypto.SigToPub(hash, signature)
	if err != nil {
		return false
	}

	recoveredAddress := crypto.PubkeyToAddress(*pubkey)
	return recoveredAddress == address
}

// hexToSignature 将十六进制签名转换为字节数组
func hexToSignature(hexStr string) ([]byte, error) {
	if len(hexStr)%2 != 0 {
		return nil, fmt.Errorf("invalid hex length")
	}

	sig := make([]byte, len(hexStr)/2)
	for i := 0; i < len(sig); i++ {
		b1 := hexCharValue(hexStr[i*2])
		b2 := hexCharValue(hexStr[i*2+1])
		if b1 == 16 || b2 == 16 {
			return nil, fmt.Errorf("invalid hex character")
		}
		sig[i] = (b1 << 4) | b2
	}

	return sig, nil
}

// hexCharValue 获取十六进制字符的值
func hexCharValue(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 16 // 无效值
	}
}

// SignTx 签名交易 (备用功能)
func (w *WalletSigner) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	signer := types.NewLondonSigner(big.NewInt(56)) // BSC chainId
	signedTx, err := types.SignTx(tx, signer, w.privateKey)
	if err != nil {
		return nil, err
	}
	return signedTx, nil
}

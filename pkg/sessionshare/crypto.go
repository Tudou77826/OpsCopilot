package sessionshare

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	// scrypt 派生参数：N=2^15 在交互频率（每次连接成功一次）下可接受，
	// 与常见推荐一致。
	scryptN     = 1 << 15
	scryptR     = 8
	scryptP     = 1
	keyLen      = 32 // AES-256
	saltLen     = 16
	nonceLen    = 12
	gcmOverhead = 16    // AES-GCM 认证标签长度
	encPrefixV1 = "v1:" // 密文版本前缀，预留算法升级空间
)

// ErrWrongKey 表示解密失败（密钥不正确或数据被篡改）。
var ErrWrongKey = errors.New("解密失败：共享密钥可能不正确")

// EncryptSecrets 用团队口令加密凭据，返回 base64(salt‖nonce‖ciphertext)，
// 带 "v1:" 版本前缀。passphrase 为空时返回错误——静默使用空密钥会让人
// 误以为已加密。
func EncryptSecrets(payload SecretsPayload, passphrase string) (string, error) {
	return encryptJSON(payload, passphrase)
}

// DecryptSecrets 解密 EncryptSecrets 的产物。认证失败（密钥错误或篡改）
// 返回 ErrWrongKey。
func DecryptSecrets(enc, passphrase string) (SecretsPayload, error) {
	var payload SecretsPayload
	if enc == "" {
		return payload, nil // 无凭据（密钥认证等场景），合法的空条目
	}
	if err := decryptJSON(enc, passphrase, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

// EncryptDatabaseSecrets 加密数据库连接凭据（含可选 SSH 隧道链路）。
func EncryptDatabaseSecrets(payload DatabaseSecretsPayload, passphrase string) (string, error) {
	return encryptJSON(payload, passphrase)
}

// DecryptDatabaseSecrets 解密 EncryptDatabaseSecrets 的产物。
func DecryptDatabaseSecrets(enc, passphrase string) (DatabaseSecretsPayload, error) {
	var payload DatabaseSecretsPayload
	if enc == "" {
		return payload, nil
	}
	if err := decryptJSON(enc, passphrase, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func encryptJSON(v interface{}, passphrase string) (string, error) {
	if strings.TrimSpace(passphrase) == "" {
		return "", fmt.Errorf("共享密钥为空，无法加密")
	}

	plaintext, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal secrets: %w", err)
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("gen salt: %w", err)
	}
	aead, err := newAEAD(passphrase, salt)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("gen nonce: %w", err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	buf := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ciphertext...)
	return encPrefixV1 + base64.StdEncoding.EncodeToString(buf), nil
}

func decryptJSON(enc, passphrase string, dst interface{}) error {
	if !strings.HasPrefix(enc, encPrefixV1) {
		return fmt.Errorf("%w（未知密文版本）", ErrWrongKey)
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, encPrefixV1))
	if err != nil {
		return fmt.Errorf("%w（base64）", ErrWrongKey)
	}
	if len(raw) < saltLen+nonceLen+gcmOverhead {
		return fmt.Errorf("%w（长度不足）", ErrWrongKey)
	}

	salt := raw[:saltLen]
	nonce := raw[saltLen : saltLen+nonceLen]
	ciphertext := raw[saltLen+nonceLen:]

	aead, err := newAEAD(passphrase, salt)
	if err != nil {
		return err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ErrWrongKey
	}

	if err := json.Unmarshal(plaintext, dst); err != nil {
		return fmt.Errorf("unmarshal secrets: %w", err)
	}
	return nil
}

func newAEAD(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

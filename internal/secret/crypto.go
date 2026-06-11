package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

const (
	defaultN      = 32768
	defaultR      = 8
	defaultP      = 1
	defaultKeyLen = 32
)

var ErrIncorrectPassphrase = errors.New("主密码错误")

// CryptoConfig holds scrypt and encryption parameters.
type CryptoConfig struct {
	N      int
	R      int
	P      int
	KeyLen int
	Salt   []byte
}

// ScryptConfig is the on-disk scrypt configuration.
type ScryptConfig struct {
	N      int `yaml:"n"`
	R      int `yaml:"r"`
	P      int `yaml:"p"`
	KeyLen int `yaml:"key_len"`
}

// EncryptedFile is the on-disk structure of secrets.yaml.
type EncryptedFile struct {
	Version       int          `yaml:"version"`
	KDF           string       `yaml:"kdf"`
	Cipher        string       `yaml:"cipher"`
	Scrypt        ScryptConfig `yaml:"scrypt"`
	SaltB64       string       `yaml:"salt"`
	NonceB64      string       `yaml:"nonce"`
	CiphertextB64 string       `yaml:"ciphertext"`
}

// deriveKey derives an encryption key from a passphrase and salt.
func deriveKey(passphrase string, salt []byte, cfg CryptoConfig) ([]byte, error) {
	if cfg.N < 2 || cfg.N&(cfg.N-1) != 0 || cfg.N > 1<<20 {
		return nil, fmt.Errorf("无效的 scrypt n 参数: %d", cfg.N)
	}
	if cfg.R < 1 || cfg.R > 32 {
		return nil, fmt.Errorf("无效的 scrypt r 参数: %d", cfg.R)
	}
	if cfg.P < 1 || cfg.P > 16 {
		return nil, fmt.Errorf("无效的 scrypt p 参数: %d", cfg.P)
	}
	if cfg.KeyLen != defaultKeyLen {
		return nil, fmt.Errorf("无效的 scrypt key_len 参数: %d", cfg.KeyLen)
	}
	return scrypt.Key([]byte(passphrase), salt, cfg.N, cfg.R, cfg.P, cfg.KeyLen)
}

// Encrypt encrypts plaintext with AES-256-GCM.
func Encrypt(plaintext string, passphrase string, salt []byte) (*EncryptedFile, error) {
	cfg := CryptoConfig{
		N:      defaultN,
		R:      defaultR,
		P:      defaultP,
		KeyLen: defaultKeyLen,
		Salt:   salt,
	}
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("生成 salt 失败: %w", err)
		}
		cfg.Salt = salt
	}

	key, err := deriveKey(passphrase, salt, cfg)
	if err != nil {
		return nil, fmt.Errorf("密钥派生失败: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %w", err)
	}

	ciphertext := aesGCM.Seal(nil, nonce, []byte(plaintext), nil)

	return &EncryptedFile{
		Version: 1,
		KDF:     "scrypt",
		Cipher:  "aes-256-gcm",
		Scrypt: ScryptConfig{
			N:      cfg.N,
			R:      cfg.R,
			P:      cfg.P,
			KeyLen: cfg.KeyLen,
		},
		SaltB64:       base64.StdEncoding.EncodeToString(salt),
		NonceB64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// Decrypt decrypts an EncryptedFile back to plaintext.
func Decrypt(ef *EncryptedFile, passphrase string) (string, error) {
	if ef.Version != 1 {
		return "", fmt.Errorf("不支持的 secrets 文件版本: %d", ef.Version)
	}
	if ef.KDF != "scrypt" {
		return "", fmt.Errorf("不支持的 KDF: %s", ef.KDF)
	}
	if ef.Cipher != "aes-256-gcm" {
		return "", fmt.Errorf("不支持的加密算法: %s", ef.Cipher)
	}

	salt, err := base64.StdEncoding.DecodeString(ef.SaltB64)
	if err != nil {
		return "", fmt.Errorf("解码 salt 失败: %w", err)
	}
	if len(salt) < 16 || len(salt) > 64 {
		return "", fmt.Errorf("无效的 salt 长度: %d", len(salt))
	}
	nonce, err := base64.StdEncoding.DecodeString(ef.NonceB64)
	if err != nil {
		return "", fmt.Errorf("解码 nonce 失败: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ef.CiphertextB64)
	if err != nil {
		return "", fmt.Errorf("解码密文失败: %w", err)
	}

	cfg := CryptoConfig{
		N:      ef.Scrypt.N,
		R:      ef.Scrypt.R,
		P:      ef.Scrypt.P,
		KeyLen: ef.Scrypt.KeyLen,
		Salt:   salt,
	}

	key, err := deriveKey(passphrase, salt, cfg)
	if err != nil {
		return "", fmt.Errorf("密钥派生失败: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 失败: %w", err)
	}
	if len(nonce) != aesGCM.NonceSize() {
		return "", fmt.Errorf("无效的 nonce 长度: %d", len(nonce))
	}

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrIncorrectPassphrase
	}

	return string(plaintext), nil
}

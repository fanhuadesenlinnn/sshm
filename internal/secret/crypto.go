package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

const (
	defaultN      = 32768
	defaultR      = 8
	defaultP      = 1
	defaultKeyLen = 32
)

// CryptoConfig holds scrypt and encryption parameters.
type CryptoConfig struct {
	N      int
	R      int
	P      int
	KeyLen int
	Salt   []byte
}

// EncryptedFile is the on-disk structure of secrets.yaml.
type EncryptedFile struct {
	Version    int    `yaml:"version"`
	KDF        string `yaml:"kdf"`
	Cipher     string `yaml:"cipher"`
	ScryptN    int    `yaml:"scrypt.n"`
	ScryptR    int    `yaml:"scrypt.r"`
	ScryptP    int    `yaml:"scrypt.p"`
	ScryptKeyLen int  `yaml:"scrypt.key_len"`
	SaltB64    string `yaml:"salt"`
	NonceB64   string `yaml:"nonce"`
	CiphertextB64 string `yaml:"ciphertext"`
}

// deriveKey derives an encryption key from a passphrase and salt.
func deriveKey(passphrase string, salt []byte, cfg CryptoConfig) ([]byte, error) {
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
		Version:       1,
		KDF:           "scrypt",
		Cipher:        "aes-256-gcm",
		ScryptN:       cfg.N,
		ScryptR:       cfg.R,
		ScryptP:       cfg.P,
		ScryptKeyLen:  cfg.KeyLen,
		SaltB64:       base64.StdEncoding.EncodeToString(salt),
		NonceB64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

// Decrypt decrypts an EncryptedFile back to plaintext.
func Decrypt(ef *EncryptedFile, passphrase string) (string, error) {
	salt, err := base64.StdEncoding.DecodeString(ef.SaltB64)
	if err != nil {
		return "", fmt.Errorf("解码 salt 失败: %w", err)
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
		N:      ef.ScryptN,
		R:      ef.ScryptR,
		P:      ef.ScryptP,
		KeyLen: ef.ScryptKeyLen,
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

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("secrets 解密失败，请确认主密码是否正确")
	}

	return string(plaintext), nil
}

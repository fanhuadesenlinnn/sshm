package keymgr

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// GenerateManagedKey creates an Ed25519 private key in memory.
func GenerateManagedKey(name string) ([]byte, string, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("生成 Ed25519 密钥失败: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(private, "sshm:"+name)
	if err != nil {
		return nil, "", fmt.Errorf("序列化私钥失败: %w", err)
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return nil, "", fmt.Errorf("序列化公钥失败: %w", err)
	}
	privatePEM := pem.EncodeToMemory(block)
	publicLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublic))) + " sshm:" + name
	return privatePEM, publicLine, nil
}

// ParseManagedKey validates an imported private key and derives its public key.
func ParseManagedKey(privatePEM []byte, passphrase []byte, name string) ([]byte, string, error) {
	var rawKey any
	var err error
	if len(passphrase) > 0 {
		rawKey, err = ssh.ParseRawPrivateKeyWithPassphrase(privatePEM, passphrase)
	} else {
		rawKey, err = ssh.ParseRawPrivateKey(privatePEM)
	}
	if err != nil {
		return nil, "", fmt.Errorf("解析 SSH 私钥失败: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(rawKey)
	if err != nil {
		return nil, "", fmt.Errorf("创建 SSH signer 失败: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(rawKey, "sshm:"+name)
	if err != nil {
		return nil, "", fmt.Errorf("标准化 SSH 私钥失败: %w", err)
	}
	publicLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " sshm:" + name
	return pem.EncodeToMemory(block), publicLine, nil
}

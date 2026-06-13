package keymgr

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/safefile"
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

// ImportKey copies a private key into the sshm keys directory.
func ImportKey(alias string, srcPath string) (string, error) {
	srcPath = config.ExpandPath(srcPath)

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return "", fmt.Errorf("私钥文件不存在: %s", srcPath)
	}

	keysDir := config.KeysDir()
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return "", fmt.Errorf("创建密钥目录失败: %w", err)
	}

	baseName := filepath.Base(srcPath)
	destName := alias + "_" + baseName
	destPath := filepath.Join(keysDir, destName)

	// Copy private key
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("读取私钥失败: %w", err)
	}
	if _, err := ssh.ParseRawPrivateKey(srcData); err != nil {
		var passphraseErr *ssh.PassphraseMissingError
		if !errors.As(err, &passphraseErr) {
			return "", fmt.Errorf("文件不是可解析的 SSH 私钥: %w", err)
		}
	}
	var pubData []byte
	pubSrcPath := srcPath + ".pub"
	if _, err := os.Stat(pubSrcPath); err == nil {
		pubData, err = os.ReadFile(pubSrcPath)
		if err != nil {
			return "", fmt.Errorf("读取公钥失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("检查公钥失败: %w", err)
	}

	if err := safefile.WithLock(destPath, func() error {
		if _, err := os.Stat(destPath); err == nil {
			return fmt.Errorf("目标密钥已存在，拒绝覆盖: %s", destPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查目标密钥失败: %w", err)
		}
		if pubData != nil {
			pubDestPath := destPath + ".pub"
			if _, err := os.Stat(pubDestPath); err == nil {
				return fmt.Errorf("目标公钥已存在，拒绝覆盖: %s", pubDestPath)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("检查目标公钥失败: %w", err)
			}
		}
		if err := safefile.Write(destPath, srcData, 0600, false); err != nil {
			return fmt.Errorf("写入私钥失败: %w", err)
		}
		if pubData != nil {
			pubDestPath := destPath + ".pub"
			if err := safefile.Write(pubDestPath, pubData, 0644, false); err != nil {
				_ = os.Remove(destPath)
				return fmt.Errorf("写入公钥失败: %w", err)
			}
		}
		return nil
	}); err != nil {
		return "", err
	}

	relativePath := "keys/" + destName
	return relativePath, nil
}

// GenerateKey creates a new ed25519 key pair for the given alias.
func GenerateKey(alias string) (string, error) {
	keysDir := config.KeysDir()
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		return "", fmt.Errorf("创建密钥目录失败: %w", err)
	}

	keyName := alias + "_ed25519"
	keyPath := filepath.Join(keysDir, keyName)

	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		return "", fmt.Errorf("密钥已存在: %s", keyPath)
	}

	cmd := exec.Command("ssh-keygen",
		"-t", "ed25519",
		"-f", keyPath,
		"-N", "",
		"-C", alias,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("生成密钥失败: %w", err)
	}

	// Ensure proper permissions
	if err := os.Chmod(keyPath, 0600); err != nil {
		return "", fmt.Errorf("设置私钥权限失败: %w", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err == nil {
		if err := os.Chmod(keyPath+".pub", 0644); err != nil {
			return "", fmt.Errorf("设置公钥权限失败: %w", err)
		}
	}

	relativePath := "keys/" + keyName
	return relativePath, nil
}

// ShowPubKey reads and returns the public key for a host entry.
func ShowPubKey(host config.Host) (string, error) {
	if host.Identity == "" {
		return "", fmt.Errorf("主机 %s 未配置密钥", host.Alias)
	}

	keyPath := config.ExpandPath(host.Identity)
	pubPath := keyPath + ".pub"

	data, err := os.ReadFile(pubPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("公钥文件不存在: %s", pubPath)
		}
		return "", fmt.Errorf("读取公钥失败: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// IsManagedKey reports whether identity is inside sshm's managed keys directory.
func IsManagedKey(identity string) bool {
	if identity == "" {
		return false
	}
	keyPath, err := filepath.Abs(config.ExpandPath(identity))
	if err != nil {
		return false
	}
	keysDir, err := filepath.Abs(config.KeysDir())
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(keysDir, keyPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RemoveManagedKey deletes only keys located inside sshm's managed keys directory.
func RemoveManagedKey(identity string) error {
	if !IsManagedKey(identity) {
		return fmt.Errorf("拒绝删除 sshm 管理目录之外的密钥")
	}
	keyPath := config.ExpandPath(identity)
	for _, path := range []string{keyPath, keyPath + ".pub"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除密钥 %s 失败: %w", path, err)
		}
	}
	return nil
}

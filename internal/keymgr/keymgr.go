package keymgr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sshm/sshm/internal/config"
)

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
	if err := os.WriteFile(destPath, srcData, 0600); err != nil {
		return "", fmt.Errorf("写入私钥失败: %w", err)
	}

	// Copy public key if exists
	pubSrcPath := srcPath + ".pub"
	if _, err := os.Stat(pubSrcPath); err == nil {
		pubData, err := os.ReadFile(pubSrcPath)
		if err == nil {
			pubDestPath := destPath + ".pub"
			if err := os.WriteFile(pubDestPath, pubData, 0644); err != nil {
				return "", fmt.Errorf("写入公钥失败: %w", err)
			}
		}
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
	os.Chmod(keyPath, 0600)
	if _, err := os.Stat(keyPath + ".pub"); err == nil {
		os.Chmod(keyPath+".pub", 0644)
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

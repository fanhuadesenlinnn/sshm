package secret

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileStore manages encrypted password storage.
type FileStore struct {
	path       string
	passphrase string
}

// NewFileStore creates a FileStore for the given path and master passphrase.
func NewFileStore(path string, passphrase string) *FileStore {
	return &FileStore{path: path, passphrase: passphrase}
}

// makeKey builds the plaintext key for a password entry.
func makeKey(ref string) string {
	return fmt.Sprintf("password:%s", ref)
}

// readRaw loads the encrypted file and returns the raw plaintext key-value lines.
func (fs *FileStore) readRaw() (map[string]string, []byte, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil, nil
		}
		return nil, nil, fmt.Errorf("读取 secrets 文件失败: %w", err)
	}

	var ef EncryptedFile
	if err := yaml.Unmarshal(data, &ef); err != nil {
		return nil, nil, fmt.Errorf("解析 secrets 文件失败: %w", err)
	}

	plain, err := Decrypt(&ef, fs.passphrase)
	if err != nil {
		return nil, nil, err
	}

	entries := map[string]string{}
	for _, line := range strings.Split(plain, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3) // "password:ref:value"
		if len(parts) == 3 && parts[0] == "password" {
			entries[parts[1]] = parts[2]
		}
	}

	return entries, data, nil
}

// GetPassword retrieves the saved SSH password for a ref (alias or ID).
func (fs *FileStore) GetPassword(ref string) (string, error) {
	entries, _, err := fs.readRaw()
	if err != nil {
		return "", err
	}
	pass, ok := entries[ref]
	if !ok {
		return "", fmt.Errorf("未找到 %s 的密码", ref)
	}
	return pass, nil
}

// SetPassword saves or replaces the SSH password for a ref.
func (fs *FileStore) SetPassword(ref string, password string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		entries[ref] = password
	})
}

// SetPasswordByID saves a password keyed by stable ID,
// and migrates an existing alias-keyed entry if present.
func (fs *FileStore) SetPasswordByID(id string, alias string, password string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		// Remove old alias-keyed entry if present
		if alias != "" && entries[alias] != "" {
			delete(entries, alias)
		}
		entries[id] = password
	})
}

// RemovePassword deletes the SSH password for a ref.
func (fs *FileStore) RemovePassword(ref string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		delete(entries, ref)
	})
}

// MigrateAliasToID re-keys a password from alias to stable ID.
// Returns true if a migration was performed.
func (fs *FileStore) MigrateAliasToID(alias string, id string) (bool, error) {
	migrated := false
	err := fs.writeSecrets(func(entries map[string]string) {
		if pass, ok := entries[alias]; ok && alias != id {
			delete(entries, alias)
			entries[id] = pass
			migrated = true
		}
	})
	return migrated, err
}

// writeSecrets serializes, encrypts, and writes the secrets file atomically.
func (fs *FileStore) writeSecrets(mutate func(map[string]string)) error {
	entries, rawData, err := fs.readRaw()
	if err != nil {
		return fmt.Errorf("无法读取现有 secrets，已拒绝覆盖: %w", err)
	}

	mutate(entries)

	// Build plaintext
	var lines []string
	for ref, pass := range entries {
		lines = append(lines, makeKey(ref)+":"+pass)
	}
	plaintext := strings.Join(lines, "\n")

	// Extract existing salt if available, otherwise generate new
	var salt []byte
	if rawData != nil {
		var ef EncryptedFile
		if err := yaml.Unmarshal(rawData, &ef); err == nil {
			salt, _ = base64.StdEncoding.DecodeString(ef.SaltB64)
		}
	}
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return fmt.Errorf("生成 salt 失败: %w", err)
		}
	}

	ef, err := Encrypt(plaintext, fs.passphrase, salt)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(ef)
	if err != nil {
		return fmt.Errorf("序列化 secrets 文件失败: %w", err)
	}

	dir := filepath.Dir(fs.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建 secrets 目录失败: %w", err)
	}

	// Atomic write: temp file + rename
	tmp, err := os.CreateTemp(dir, ".secrets-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, fs.path); err != nil {
		return fmt.Errorf("原子替换 secrets 文件失败: %w", err)
	}

	return nil
}

// VerifyPassphrase checks whether the master passphrase is correct.
func (fs *FileStore) VerifyPassphrase() error {
	_, _, err := fs.readRaw()
	return err
}

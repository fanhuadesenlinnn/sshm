package secret

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/safefile"
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

func passwordKey(ref string) string {
	return fmt.Sprintf("password:%s", ref)
}

func managedKey(name string) string {
	return fmt.Sprintf("managed-key:%s", name)
}

// readRaw loads the encrypted file and returns namespaced plaintext entries.
func (fs *FileStore) readRaw() (map[string]string, []byte, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil, nil
		}
		return nil, nil, fmt.Errorf("读取 secrets 文件失败（可检查备份 %s.bak）: %w", fs.path, err)
	}

	var ef EncryptedFile
	if err := yaml.Unmarshal(data, &ef); err != nil {
		return nil, nil, fmt.Errorf("解析 secrets 文件失败（主文件未修改，可检查备份 %s.bak）: %w", fs.path, err)
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
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && (parts[0] == "password" || parts[0] == "managed-key") {
			entries[parts[0]+":"+parts[1]] = parts[2]
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
	pass, ok := entries[passwordKey(ref)]
	if !ok {
		return "", fmt.Errorf("未找到 %s 的密码", ref)
	}
	return pass, nil
}

// SetPassword saves or replaces the SSH password for a ref.
func (fs *FileStore) SetPassword(ref string, password string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		entries[passwordKey(ref)] = password
	})
}

// SetPasswordByID saves a password keyed by stable ID,
// and migrates an existing alias-keyed entry if present.
func (fs *FileStore) SetPasswordByID(id string, alias string, password string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		if alias != "" {
			delete(entries, passwordKey(alias))
		}
		entries[passwordKey(id)] = password
	})
}

// RemovePassword deletes the SSH password for a ref.
func (fs *FileStore) RemovePassword(ref string) error {
	return fs.RemovePasswords(ref)
}

// RemovePasswords deletes multiple references in one encrypted-file transaction.
func (fs *FileStore) RemovePasswords(refs ...string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		for _, ref := range refs {
			delete(entries, passwordKey(ref))
		}
	})
}

// MigrateAliasToID re-keys a password from alias to stable ID.
// Returns true if a migration was performed.
func (fs *FileStore) MigrateAliasToID(alias string, id string) (bool, error) {
	migrated := false
	err := fs.writeSecrets(func(entries map[string]string) {
		if pass, ok := entries[passwordKey(alias)]; ok && alias != id {
			delete(entries, passwordKey(alias))
			entries[passwordKey(id)] = pass
			migrated = true
		}
	})
	return migrated, err
}

// CopyPasswords copies old references to stable IDs without deleting the old
// references. Callers can safely update hosts.yaml before cleaning up old keys.
func (fs *FileStore) CopyPasswords(destToSource map[string]string) error {
	return fs.writeSecretsE(func(entries map[string]string) error {
		for dest, source := range destToSource {
			pass, ok := entries[passwordKey(source)]
			if !ok {
				return fmt.Errorf("未找到旧密码引用 %s", source)
			}
			entries[passwordKey(dest)] = pass
		}
		return nil
	})
}

// writeSecrets serializes, encrypts, and writes the secrets file atomically.
func (fs *FileStore) writeSecrets(mutate func(map[string]string)) error {
	return fs.writeSecretsE(func(entries map[string]string) error {
		mutate(entries)
		return nil
	})
}

func (fs *FileStore) writeSecretsE(mutate func(map[string]string) error) error {
	return safefile.WithLock(fs.path, func() error {
		entries, rawData, err := fs.readRaw()
		if err != nil {
			return fmt.Errorf("无法读取现有 secrets，已拒绝覆盖: %w", err)
		}

		if err := mutate(entries); err != nil {
			return err
		}

		keys := make([]string, 0, len(entries))
		for key := range entries {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, key+":"+entries[key])
		}
		plaintext := strings.Join(lines, "\n")

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
		return safefile.Write(fs.path, data, 0600, true)
	})
}

// GetManagedKey retrieves a managed private key.
func (fs *FileStore) GetManagedKey(name string) ([]byte, error) {
	entries, _, err := fs.readRaw()
	if err != nil {
		return nil, err
	}
	value, ok := entries[managedKey(name)]
	if !ok {
		return nil, fmt.Errorf("未找到托管密钥: %s", name)
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("解码托管密钥 %s 失败: %w", name, err)
	}
	return data, nil
}

// SetManagedKey saves a private key protected by the sshm master password.
func (fs *FileStore) SetManagedKey(name string, privateKey []byte) error {
	value := base64.StdEncoding.EncodeToString(privateKey)
	return fs.writeSecrets(func(entries map[string]string) {
		entries[managedKey(name)] = value
	})
}

// RemoveManagedKeys deletes private keys in one encrypted-file transaction.
func (fs *FileStore) RemoveManagedKeys(names ...string) error {
	return fs.writeSecrets(func(entries map[string]string) {
		for _, name := range names {
			delete(entries, managedKey(name))
		}
	})
}

// VerifyPassphrase checks whether the master passphrase is correct.
func (fs *FileStore) VerifyPassphrase() error {
	_, _, err := fs.readRaw()
	return err
}

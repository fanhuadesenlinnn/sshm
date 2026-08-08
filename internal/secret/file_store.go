package secret

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

// FileStore is a credential-focused view over the vault in sshmd.yaml.
type FileStore struct {
	path       string
	passphrase string
	repo       *config.Repository
}

func NewFileStore(path string, passphrase string) *FileStore {
	return &FileStore{
		path:       path,
		passphrase: passphrase,
		repo:       config.NewRepositoryWithPath(path),
	}
}

func VaultExists(path string) (bool, error) {
	doc, err := config.NewRepositoryWithPath(path).Load()
	if err != nil {
		return false, err
	}
	return doc.Vault != nil, nil
}

func passwordKey(ref string) string { return "password:" + ref }
func managedKey(name string) string { return "managed-key:" + name }

func decodeEntries(vault *config.EncryptedVault, passphrase string) (map[string]string, error) {
	if vault == nil {
		return map[string]string{}, nil
	}
	plain, err := Decrypt(vault, passphrase)
	if err != nil {
		return nil, err
	}
	entries := map[string]string{}
	if err := json.Unmarshal([]byte(plain), &entries); err != nil {
		return nil, fmt.Errorf("解析 vault 内容失败: %w", err)
	}
	return entries, nil
}

func encodeEntries(entries map[string]string, passphrase string, old *config.EncryptedVault) (*config.EncryptedVault, error) {
	plain, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("序列化 vault 内容失败: %w", err)
	}
	var salt []byte
	if old != nil {
		salt, _ = base64.StdEncoding.DecodeString(old.SaltB64)
	}
	if salt == nil {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("生成 salt 失败: %w", err)
		}
	}
	return Encrypt(string(plain), passphrase, salt)
}

func (fs *FileStore) readRaw() (map[string]string, *config.EncryptedVault, error) {
	doc, err := fs.repo.Load()
	if err != nil {
		return nil, nil, err
	}
	entries, err := decodeEntries(doc.Vault, fs.passphrase)
	if err != nil {
		return nil, nil, err
	}
	return entries, doc.Vault, err
}

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

// GetPasswordForHost resolves a host's SSH password from either its plaintext
// config field or the encrypted vault, in that order.
func (fs *FileStore) GetPasswordForHost(host config.Host) (string, error) {
	if host.Password != "" {
		return host.Password, nil
	}
	if host.PasswordRef != "" {
		return fs.GetPassword(host.PasswordRef)
	}
	return "", fmt.Errorf("主机 %s 未配置密码", host.Alias)
}

func (fs *FileStore) SetPassword(ref, password string) error {
	return fs.writeSecrets(func(entries map[string]string) error {
		entries[passwordKey(ref)] = password
		return nil
	})
}

func (fs *FileStore) writeSecrets(mutate func(map[string]string) error) error {
	return fs.UpdateDocument(func(_ *config.Document, entries map[string]string) error {
		return mutate(entries)
	})
}

// UpdateDocument atomically changes public configuration and encrypted entries.
func (fs *FileStore) UpdateDocument(mutate func(*config.Document, map[string]string) error) error {
	return fs.repo.Update(func(doc *config.Document) error {
		entries, err := decodeEntries(doc.Vault, fs.passphrase)
		if err != nil {
			return fmt.Errorf("无法读取现有 vault，已拒绝覆盖: %w", err)
		}
		if err := mutate(doc, entries); err != nil {
			return err
		}
		doc.Vault, err = encodeEntries(entries, fs.passphrase, doc.Vault)
		return err
	})
}

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

func (fs *FileStore) SetManagedKey(name string, privateKey []byte) error {
	value := base64.StdEncoding.EncodeToString(privateKey)
	return fs.writeSecrets(func(entries map[string]string) error {
		entries[managedKey(name)] = value
		return nil
	})
}

func (fs *FileStore) RemoveManagedKeys(names ...string) error {
	return fs.writeSecrets(func(entries map[string]string) error {
		for _, name := range names {
			delete(entries, managedKey(name))
		}
		return nil
	})
}

func (fs *FileStore) VerifyPassphrase() error {
	_, _, err := fs.readRaw()
	return err
}

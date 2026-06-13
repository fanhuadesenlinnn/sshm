package config

import (
	"fmt"
	"os"

	"github.com/fanhuadesenlinnn/sshm/internal/safefile"
	"gopkg.in/yaml.v3"
)

const managedKeysVersion = 1

// KeyStore manages metadata for master-password-protected SSH keys.
type KeyStore struct {
	path string
}

// NewKeyStore creates a KeyStore using the default keys.yaml path.
func NewKeyStore() *KeyStore {
	return &KeyStore{path: ManagedKeysFilePath()}
}

// NewKeyStoreWithPath creates a KeyStore with an explicit file path.
func NewKeyStoreWithPath(path string) *KeyStore {
	return &KeyStore{path: path}
}

// Path returns the metadata file path.
func (s *KeyStore) Path() string { return s.path }

// Load reads managed key metadata.
func (s *KeyStore) Load() (*ManagedKeysFile, error) {
	var kf *ManagedKeysFile
	err := safefile.WithLock(s.path, func() error {
		var err error
		kf, err = s.loadUnlocked()
		return err
	})
	return kf, err
}

func (s *KeyStore) loadUnlocked() (*ManagedKeysFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ManagedKeysFile{Version: managedKeysVersion, Keys: []ManagedKey{}}, nil
		}
		return nil, fmt.Errorf("读取托管密钥元数据失败（可检查备份 %s.bak）: %w", s.path, err)
	}
	var kf ManagedKeysFile
	if err := yaml.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("解析托管密钥元数据失败（主文件未修改，可检查备份 %s.bak）: %w", s.path, err)
	}
	kf.Version = managedKeysVersion
	kf.normalize()
	if kf.Default != "" {
		if _, ok := findManagedKey(&kf, kf.Default); !ok {
			return nil, fmt.Errorf("默认密钥 %q 不存在", kf.Default)
		}
	}
	return &kf, nil
}

// Save writes managed key metadata atomically.
func (s *KeyStore) Save(kf *ManagedKeysFile) error {
	return safefile.WithLock(s.path, func() error {
		return s.saveUnlocked(kf)
	})
}

func (s *KeyStore) saveUnlocked(kf *ManagedKeysFile) error {
	kf.Version = managedKeysVersion
	kf.normalize()
	seen := map[string]bool{}
	for _, key := range kf.Keys {
		if err := ValidateManagedKeyName(key.Name); err != nil {
			return fmt.Errorf("无效的托管密钥 %q: %w", key.Name, err)
		}
		if seen[key.Name] {
			return fmt.Errorf("托管密钥名称重复: %s", key.Name)
		}
		seen[key.Name] = true
	}
	if kf.Default != "" && !seen[kf.Default] {
		return fmt.Errorf("默认密钥 %q 不存在", kf.Default)
	}
	data, err := yaml.Marshal(kf)
	if err != nil {
		return fmt.Errorf("序列化托管密钥元数据失败: %w", err)
	}
	return safefile.Write(s.path, data, 0600, true)
}

// Add records a newly encrypted managed key without overwriting an existing key.
func (s *KeyStore) Add(name, publicKey string, makeDefault bool) error {
	if err := ValidateManagedKeyName(name); err != nil {
		return err
	}
	return safefile.WithLock(s.path, func() error {
		kf, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if _, ok := findManagedKey(kf, name); ok {
			return fmt.Errorf("托管密钥 %q 已存在，拒绝覆盖", name)
		}
		kf.Keys = append(kf.Keys, newManagedKey(name, publicKey))
		if makeDefault || kf.Default == "" {
			kf.Default = name
		}
		return s.saveUnlocked(kf)
	})
}

// Find resolves a key name or "default".
func (s *KeyStore) Find(name string) (*ManagedKey, error) {
	kf, err := s.Load()
	if err != nil {
		return nil, err
	}
	if name == "" || name == "default" {
		name = kf.Default
	}
	if name == "" {
		return nil, fmt.Errorf("尚未设置默认托管密钥")
	}
	key, ok := findManagedKey(kf, name)
	if !ok {
		return nil, fmt.Errorf("未找到托管密钥: %s", name)
	}
	copy := *key
	return &copy, nil
}

// SetDefault changes the default managed key.
func (s *KeyStore) SetDefault(name string) error {
	return safefile.WithLock(s.path, func() error {
		kf, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if name == "" {
			kf.Default = ""
			return s.saveUnlocked(kf)
		}
		if _, ok := findManagedKey(kf, name); !ok {
			return fmt.Errorf("未找到托管密钥: %s", name)
		}
		kf.Default = name
		return s.saveUnlocked(kf)
	})
}

// Remove deletes metadata for keys and refuses to delete the current default.
func (s *KeyStore) Remove(names ...string) error {
	remove := map[string]bool{}
	for _, name := range names {
		remove[name] = true
	}
	return safefile.WithLock(s.path, func() error {
		kf, err := s.loadUnlocked()
		if err != nil {
			return err
		}
		if remove[kf.Default] {
			return fmt.Errorf("不能删除默认密钥 %q，请先设置其他默认密钥", kf.Default)
		}
		found := map[string]bool{}
		filtered := kf.Keys[:0]
		for _, key := range kf.Keys {
			if remove[key.Name] {
				found[key.Name] = true
			} else {
				filtered = append(filtered, key)
			}
		}
		for name := range remove {
			if !found[name] {
				return fmt.Errorf("未找到托管密钥: %s", name)
			}
		}
		kf.Keys = filtered
		return s.saveUnlocked(kf)
	})
}

func findManagedKey(kf *ManagedKeysFile, name string) (*ManagedKey, bool) {
	for i := range kf.Keys {
		if kf.Keys[i].Name == name {
			return &kf.Keys[i], true
		}
	}
	return nil, false
}

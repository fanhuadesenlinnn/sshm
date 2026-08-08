package config

import "fmt"

// KeyStore is a managed-key-focused view over the single sshmd.yaml repository.
type KeyStore struct {
	repo *Repository
}

func NewKeyStore() *KeyStore {
	return &KeyStore{repo: NewRepository()}
}

func NewKeyStoreWithPath(path string) *KeyStore {
	return &KeyStore{repo: NewRepositoryWithPath(path)}
}

func (s *KeyStore) Path() string { return s.repo.Path() }

func (s *KeyStore) Load() (*ManagedKeysFile, error) {
	doc, err := s.repo.Load()
	if err != nil {
		return nil, err
	}
	kf := doc.ManagedKeys
	kf.Keys = append([]ManagedKey(nil), kf.Keys...)
	return &kf, nil
}

func (s *KeyStore) Save(kf *ManagedKeysFile) error {
	copy := *kf
	copy.Keys = append([]ManagedKey(nil), kf.Keys...)
	return s.repo.Update(func(doc *Document) error {
		doc.ManagedKeys = copy
		return nil
	})
}

func (s *KeyStore) Add(name, publicKey string, makeDefault bool) error {
	if err := ValidateManagedKeyName(name); err != nil {
		return err
	}
	return s.repo.Update(func(doc *Document) error {
		if _, ok := findManagedKey(&doc.ManagedKeys, name); ok {
			return fmt.Errorf("托管密钥 %q 已存在，拒绝覆盖", name)
		}
		doc.ManagedKeys.Keys = append(doc.ManagedKeys.Keys, newManagedKey(name, publicKey))
		if makeDefault || doc.ManagedKeys.Default == "" {
			doc.ManagedKeys.Default = name
		}
		return nil
	})
}

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

func (s *KeyStore) SetDefault(name string) error {
	return s.repo.Update(func(doc *Document) error {
		if name != "" {
			if _, ok := findManagedKey(&doc.ManagedKeys, name); !ok {
				return fmt.Errorf("未找到托管密钥: %s", name)
			}
		}
		doc.ManagedKeys.Default = name
		return nil
	})
}

func (s *KeyStore) Remove(names ...string) error {
	remove := map[string]bool{}
	for _, name := range names {
		remove[name] = true
	}
	return s.repo.Update(func(doc *Document) error {
		if remove[doc.ManagedKeys.Default] {
			return fmt.Errorf("不能删除默认密钥 %q，请先设置其他默认密钥", doc.ManagedKeys.Default)
		}
		found := map[string]bool{}
		filtered := doc.ManagedKeys.Keys[:0]
		for _, key := range doc.ManagedKeys.Keys {
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
		doc.ManagedKeys.Keys = filtered
		return nil
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

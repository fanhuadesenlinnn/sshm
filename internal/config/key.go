package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const managedIdentityPrefix = "managed:"

var managedKeyNameRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ManagedKey describes a private key encrypted in the sshm secret store.
type ManagedKey struct {
	Name      string `yaml:"name"`
	PublicKey string `yaml:"public_key"`
	CreatedAt string `yaml:"created_at"`
}

// ManagedKeysFile is the top-level structure of keys.yaml.
type ManagedKeysFile struct {
	Version int          `yaml:"version"`
	Default string       `yaml:"default,omitempty"`
	Keys    []ManagedKey `yaml:"keys"`
}

// ManagedIdentity returns the host identity reference for a managed key.
func ManagedIdentity(name string) string {
	return managedIdentityPrefix + name
}

// ManagedKeyName returns a managed key name and whether identity is managed.
func ManagedKeyName(identity string) (string, bool) {
	if !strings.HasPrefix(identity, managedIdentityPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(identity, managedIdentityPrefix)
	return name, name != ""
}

// ValidateManagedKeyName checks names used in commands and identity references.
func ValidateManagedKeyName(name string) error {
	if name == "" {
		return fmt.Errorf("密钥名称不能为空")
	}
	if name == "default" {
		return fmt.Errorf("密钥名称 default 为保留名称，请使用其他名称")
	}
	if !managedKeyNameRegexp.MatchString(name) {
		return fmt.Errorf("密钥名称只能包含字母、数字、点、下划线、短横线")
	}
	return nil
}

func (kf *ManagedKeysFile) normalize() {
	if kf.Keys == nil {
		kf.Keys = []ManagedKey{}
	}
	sort.SliceStable(kf.Keys, func(i, j int) bool {
		return kf.Keys[i].Name < kf.Keys[j].Name
	})
}

func newManagedKey(name, publicKey string) ManagedKey {
	return ManagedKey{
		Name:      name,
		PublicKey: strings.TrimSpace(publicKey),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}

package config

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/safefile"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

const DocumentVersion = 1

// Defaults contains global host defaults. Empty per-host values inherit these.
type Defaults struct {
	HostKeyPolicy string `yaml:"host_key_policy"`
}

// HostTrustEntry records a trusted SSH host key owned by sshm.
type HostTrustEntry struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	PublicKey string `yaml:"public_key"`
}

// HostTrust contains SSH host identities confirmed by the user.
type HostTrust struct {
	Entries []HostTrustEntry `yaml:"entries"`
}

// EncryptedVault is the encrypted on-disk credential payload.
type EncryptedVault struct {
	Version       int          `yaml:"version"`
	KDF           string       `yaml:"kdf"`
	Cipher        string       `yaml:"cipher"`
	Scrypt        ScryptConfig `yaml:"scrypt"`
	SaltB64       string       `yaml:"salt"`
	NonceB64      string       `yaml:"nonce"`
	CiphertextB64 string       `yaml:"ciphertext"`
}

// ScryptConfig is the persisted key-derivation configuration.
type ScryptConfig struct {
	N      int `yaml:"n"`
	R      int `yaml:"r"`
	P      int `yaml:"p"`
	KeyLen int `yaml:"key_len"`
}

// Document is the only persistent configuration owned by sshm.
type Document struct {
	Version     int             `yaml:"version"`
	Defaults    Defaults        `yaml:"defaults"`
	Tags        TagsFile        `yaml:"tags"`
	Hosts       []Host          `yaml:"hosts"`
	ManagedKeys ManagedKeysFile `yaml:"managed_keys"`
	HostTrust   HostTrust       `yaml:"host_trust"`
	Vault       *EncryptedVault `yaml:"vault,omitempty"`
}

// Repository atomically reads and updates sshm.yaml.
type Repository struct {
	path string
}

func NewRepository() *Repository {
	return &Repository{path: ConfigFilePath()}
}

func NewRepositoryWithPath(path string) *Repository {
	return &Repository{path: path}
}

func (r *Repository) Path() string { return r.path }

func DefaultDocument() *Document {
	return &Document{
		Version:  DocumentVersion,
		Defaults: Defaults{HostKeyPolicy: HostKeyPolicyStrict},
		Tags:     TagsFile{Items: []Tag{}},
		Hosts:    []Host{},
		ManagedKeys: ManagedKeysFile{
			Keys: []ManagedKey{},
		},
		HostTrust: HostTrust{Entries: []HostTrustEntry{}},
	}
}

func (r *Repository) Load() (*Document, error) {
	var doc *Document
	err := safefile.WithLock(r.path, func() error {
		var err error
		doc, err = r.loadUnlocked(true)
		return err
	})
	return doc, err
}

// Update runs one read-modify-write transaction against the complete document.
func (r *Repository) Update(mutate func(*Document) error) error {
	return safefile.WithLock(r.path, func() error {
		doc, err := r.loadUnlocked(false)
		if err != nil {
			return err
		}
		if err := mutate(doc); err != nil {
			return err
		}
		return r.saveUnlocked(doc)
	})
}

func (r *Repository) Replace(doc *Document) error {
	return safefile.WithLock(r.path, func() error {
		return r.saveUnlocked(doc)
	})
}

func ValidateDocumentData(data []byte) (*Document, error) {
	var doc Document
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if err := normalizeAndValidateDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *Repository) loadUnlocked(create bool) (*Document, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		doc := DefaultDocument()
		if create {
			if err := r.saveUnlocked(doc); err != nil {
				return nil, err
			}
		}
		return doc, nil
	}

	var doc Document
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析配置文件失败，原文件未修改: %w", err)
	}
	beforeNormalization, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("检查配置规范化状态失败: %w", err)
	}
	if err := normalizeAndValidateDocument(&doc); err != nil {
		return nil, err
	}
	afterNormalization, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("检查配置规范化状态失败: %w", err)
	}
	if create && !bytes.Equal(beforeNormalization, afterNormalization) {
		if err := r.saveUnlocked(&doc); err != nil {
			return nil, fmt.Errorf("保存自动补全的配置字段失败: %w", err)
		}
	}
	return &doc, nil
}

func (r *Repository) saveUnlocked(doc *Document) error {
	if err := normalizeAndValidateDocument(doc); err != nil {
		return err
	}
	data, err := encodeDocument(doc)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return safefile.Write(r.path, data, 0600)
}

func normalizeAndValidateDocument(doc *Document) error {
	if doc.Version == 0 {
		doc.Version = DocumentVersion
	}
	if doc.Version != DocumentVersion {
		return fmt.Errorf("不支持的配置版本 %d，当前仅支持 %d", doc.Version, DocumentVersion)
	}
	if doc.Defaults.HostKeyPolicy == "" {
		doc.Defaults.HostKeyPolicy = HostKeyPolicyStrict
	}
	if !ValidHostKeyPolicy(doc.Defaults.HostKeyPolicy) {
		return fmt.Errorf("无效的全局主机信任策略: %s", doc.Defaults.HostKeyPolicy)
	}
	if doc.Hosts == nil {
		doc.Hosts = []Host{}
	}
	if doc.Tags.Items == nil {
		doc.Tags.Items = []Tag{}
	}
	if doc.ManagedKeys.Keys == nil {
		doc.ManagedKeys.Keys = []ManagedKey{}
	}
	if doc.HostTrust.Entries == nil {
		doc.HostTrust.Entries = []HostTrustEntry{}
	}
	doc.ManagedKeys.normalize()
	doc.Tags.normalize()

	tagNames := map[string]bool{}
	for _, tag := range doc.Tags.Items {
		if err := ValidateTagName(tag.Name); err != nil {
			return err
		}
		if tagNames[tag.Name] {
			return fmt.Errorf("标签名称重复: %s", tag.Name)
		}
		tagNames[tag.Name] = true
	}

	aliases := map[string]bool{}
	hostsByAlias := map[string]*Host{}
	ids := map[string]bool{}
	for i := range doc.Hosts {
		host := &doc.Hosts[i]
		if host.ID == "" {
			host.ID = NewID()
		}
		host.EnsureDefaults()
		for _, tag := range host.Tags {
			if err := ValidateTagName(tag); err != nil {
				return fmt.Errorf("主机 %s 的标签无效: %w", host.Alias, err)
			}
		}
		doc.Tags.Ensure(host.Tags...)
		if aliases[host.Alias] {
			return fmt.Errorf("配置包含重复别名: %s", host.Alias)
		}
		if ids[host.ID] {
			return fmt.Errorf("配置包含重复稳定 ID: %s", host.ID)
		}
		aliases[host.Alias] = true
		hostsByAlias[host.Alias] = host
		ids[host.ID] = true
		if errs := host.Validate(); len(errs) > 0 {
			return fmt.Errorf("主机 %s 配置无效: %v", host.Alias, errs)
		}
	}

	keyNames := map[string]bool{}
	for _, key := range doc.ManagedKeys.Keys {
		if err := ValidateManagedKeyName(key.Name); err != nil {
			return err
		}
		if keyNames[key.Name] {
			return fmt.Errorf("托管密钥名称重复: %s", key.Name)
		}
		keyNames[key.Name] = true
	}
	if doc.ManagedKeys.Default != "" && !keyNames[doc.ManagedKeys.Default] {
		return fmt.Errorf("默认密钥 %q 不存在", doc.ManagedKeys.Default)
	}
	for _, host := range doc.Hosts {
		if name, ok := ManagedKeyName(host.Identity); ok && !keyNames[name] {
			return fmt.Errorf("主机 %s 引用了不存在的托管密钥 %s", host.Alias, name)
		}
		if host.JumpHost != "" && !aliases[host.JumpHost] {
			return fmt.Errorf("主机 %s 引用了不存在的跳板机 %s", host.Alias, host.JumpHost)
		}
		if host.JumpHost == host.Alias {
			return fmt.Errorf("主机 %s 不能将自身作为跳板机", host.Alias)
		}
		if host.JumpHost != "" && hostsByAlias[host.JumpHost].JumpHost != "" {
			return fmt.Errorf("仅支持单级跳板机，%s 不能引用同样使用跳板机的 %s", host.Alias, host.JumpHost)
		}
	}
	sort.SliceStable(doc.HostTrust.Entries, func(i, j int) bool {
		if doc.HostTrust.Entries[i].Host != doc.HostTrust.Entries[j].Host {
			return doc.HostTrust.Entries[i].Host < doc.HostTrust.Entries[j].Host
		}
		return doc.HostTrust.Entries[i].Port < doc.HostTrust.Entries[j].Port
	})
	trusted := map[string]bool{}
	for _, entry := range doc.HostTrust.Entries {
		key := fmt.Sprintf("%s:%d", entry.Host, entry.Port)
		if entry.Host == "" || entry.Port < 1 || entry.Port > 65535 {
			return fmt.Errorf("无效的主机信任条目: %s", key)
		}
		if trusted[key] {
			return fmt.Errorf("重复的主机信任条目: %s", key)
		}
		trusted[key] = true
		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(entry.PublicKey)); err != nil {
			return fmt.Errorf("主机信任条目 %s 的公钥无效: %w", key, err)
		}
	}
	return nil
}

func encodeDocument(doc *Document) ([]byte, error) {
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	header := `# sshm 配置；完整说明: sshm help config
# host_key_policy: strict | accept-new | insecure(跳过验证)；主机空值继承全局
# 主机必填: alias, user, host；常用可选: port, auth, identity, tags
# tags.items 可填写标签备注；主机引用的新标签会自动登记
# auth: auto | key | password；identity 填托管密钥名
# 高级可选: host_key_policy, jump_host
# deploy 工作流使用独立 deploy.yaml、deploy.d/*.yaml 或项目 sshm.deploy.yaml
# host_trust 与 vault 由 sshm 管理，请勿手动修改
`
	return append([]byte(header), body...), nil
}

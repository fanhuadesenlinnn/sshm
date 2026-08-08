package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
)

const DocumentVersion = 2

var ErrNotInitialized = errors.New("sshm 尚未初始化；请先运行 sshm init")

// Defaults contains global operation defaults.
type Defaults struct {
	HostKeyPolicy string           `yaml:"host_key_policy"`
	Batch         BatchDefaults    `yaml:"batch"`
	Exec          ExecDefaults     `yaml:"exec"`
	Transfer      TransferDefaults `yaml:"transfer"`
	Logs          LogDefaults      `yaml:"logs"`
}

type BatchDefaults struct {
	Parallel       int      `yaml:"parallel"`
	ConnectTimeout Duration `yaml:"connect_timeout"`
}

type ExecDefaults struct {
	Timeout Duration `yaml:"timeout"`
}

type TransferDefaults struct {
	Timeout Duration `yaml:"timeout"`
}

type LogDefaults struct {
	Enabled    bool     `yaml:"enabled"`
	Retention  Duration `yaml:"retention"`
	enabledSet bool
}

func (d *LogDefaults) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("defaults.logs 必须是映射")
	}
	for index := 0; index < len(node.Content); index += 2 {
		switch node.Content[index].Value {
		case "enabled", "retention":
		default:
			return fmt.Errorf("defaults.logs 包含未知字段 %q", node.Content[index].Value)
		}
	}
	type rawLogDefaults struct {
		Enabled   *bool    `yaml:"enabled"`
		Retention Duration `yaml:"retention"`
	}
	var raw rawLogDefaults
	if err := node.Decode(&raw); err != nil {
		return err
	}
	d.Retention = raw.Retention
	if raw.Enabled != nil {
		d.Enabled = *raw.Enabled
		d.enabledSet = true
	}
	return nil
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
	Vault       *EncryptedVault `yaml:"vault"`
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
		Version: DocumentVersion,
		Defaults: Defaults{
			HostKeyPolicy: HostKeyPolicyStrict,
			Batch: BatchDefaults{
				Parallel:       4,
				ConnectTimeout: Duration{Duration: 10 * time.Second},
			},
			Exec: ExecDefaults{
				Timeout: Duration{Duration: 30 * time.Second},
			},
			Transfer: TransferDefaults{
				Timeout: Duration{Duration: 15 * time.Minute},
			},
			Logs: LogDefaults{
				Enabled:    true,
				Retention:  Duration{Duration: 30 * 24 * time.Hour},
				enabledSet: true,
			},
		},
		Tags:  TagsFile{Items: []Tag{}},
		Hosts: []Host{},
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
		doc, err = r.loadUnlocked()
		return err
	})
	return doc, err
}

// Update runs one read-modify-write transaction against the complete document.
func (r *Repository) Update(mutate func(*Document) error) error {
	return safefile.WithLock(r.path, func() error {
		doc, err := r.loadUnlocked()
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

func (r *Repository) loadUnlocked() (*Document, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		return nil, ErrNotInitialized
	}

	var doc Document
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析配置文件失败，原文件未修改: %w", err)
	}
	idsAdded := ensureDocumentHostIDs(&doc)
	if err := normalizeAndValidateDocument(&doc); err != nil {
		return nil, err
	}
	if idsAdded {
		data, err := encodeDocument(&doc)
		if err != nil {
			return nil, fmt.Errorf("序列化自动补全后的配置失败: %w", err)
		}
		if err := safefile.Write(r.path, data, 0600); err != nil {
			return nil, fmt.Errorf("保存自动生成的主机 ID 失败: %w", err)
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
		return fmt.Errorf("配置缺少必填字段 version")
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
	if doc.Defaults.Batch.Parallel == 0 {
		doc.Defaults.Batch.Parallel = 4
	}
	if doc.Defaults.Batch.Parallel < 1 || doc.Defaults.Batch.Parallel > 128 {
		return fmt.Errorf("defaults.batch.parallel 必须在 1 到 128 之间")
	}
	if doc.Defaults.Batch.ConnectTimeout.Duration == 0 {
		doc.Defaults.Batch.ConnectTimeout.Duration = 10 * time.Second
	}
	if doc.Defaults.Exec.Timeout.Duration == 0 {
		doc.Defaults.Exec.Timeout.Duration = 30 * time.Second
	}
	if doc.Defaults.Transfer.Timeout.Duration == 0 {
		doc.Defaults.Transfer.Timeout.Duration = 15 * time.Minute
	}
	if doc.Defaults.Logs.Retention.Duration == 0 {
		doc.Defaults.Logs.Retention.Duration = 30 * 24 * time.Hour
	}
	if !doc.Defaults.Logs.enabledSet {
		doc.Defaults.Logs.Enabled = true
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
	ensureDocumentHostIDs(doc)

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

// ensureDocumentHostIDs assigns stable IDs to hosts added by hand while
// preserving every existing ID. Generated IDs are checked against explicit
// IDs in the same document before they are assigned.
func ensureDocumentHostIDs(doc *Document) bool {
	used := make(map[string]bool, len(doc.Hosts))
	for _, host := range doc.Hosts {
		if host.ID != "" {
			used[host.ID] = true
		}
	}
	changed := false
	for i := range doc.Hosts {
		if doc.Hosts[i].ID != "" {
			continue
		}
		id := NewID()
		for used[id] {
			id = NewID()
		}
		doc.Hosts[i].ID = id
		used[id] = true
		changed = true
	}
	return changed
}

func encodeDocument(doc *Document) ([]byte, error) {
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	annotateDocument(&root)
	var rendered bytes.Buffer
	encoder := yaml.NewEncoder(&rendered)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	header := `# sshm 配置文件
#
# 快速开始：
#   添加主机：sshm add web01 root@10.0.0.11 --tags prod,web
#   保存密码：sshm passwd web01
#   测试连接：sshm ping web01
#   连接主机：sshm web01
#
# 默认数据目录是 ~/.sshm；设置 SSHM_HOME 后，所有路径改用指定目录。
# Deploy 编排请编辑同目录的 deploy.yaml 或 deploy.d/*.yaml。
#
# 主机密钥策略：
#   strict      首次连接需要确认，主机密钥变化会被拒绝
#   accept-new  新主机自动信任，主机密钥变化会被拒绝
#   insecure    跳过主机密钥校验，不推荐
#
# 也可以手工添加主机，把 hosts: [] 替换为：
# hosts:
#   - alias: web01
#     user: root
#     host: 10.0.0.11
#     port: 22
#     auth: auto
#     tags: [prod, web]
#     note: 生产 Web 服务器
#
# 手工新增 hosts 条目时可以省略 id，sshm 校验后会自动生成并写回
# 已有主机的 id 用于关联凭据，请勿修改
# 密码两种方式：sshm passwd 加密进 vault（推荐）；或显式写 password 字段（明文，受 0600 保护）
# managed_keys、host_trust 与 vault 由 sshm 管理，请勿手动编辑
# sshm 保存时会规范化 YAML；自定义说明请写入主机或标签的 note 字段
`
	return append([]byte(header), rendered.Bytes()...), nil
}

func annotateDocument(root *yaml.Node) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}
	document := root.Content[0]
	setMappingHeadComment(document, "version", "配置格式版本；当前必须是 2。")
	setMappingHeadComment(document, "defaults", "全局默认值；单台主机可以覆盖主机信任策略。")
	setMappingHeadComment(document, "tags", "标签定义；主机引用新标签时 sshm 会自动登记。")
	setMappingHeadComment(document, "hosts", "主机列表；推荐使用 sshm add，也支持按文件顶部示例手工编辑。")
	setMappingHeadComment(document, "managed_keys", "sshm 托管的密钥元数据；请通过 sshm key 命令维护。")
	setMappingHeadComment(document, "host_trust", "已确认的 SSH 主机公钥；由 sshm 自动维护，请勿手工修改。")
	setMappingHeadComment(document, "vault", "加密凭据数据；由 sshm 自动维护，绝不能改成明文密码。")

	defaults := mappingValue(document, "defaults")
	setMappingHeadComment(defaults, "host_key_policy", "主机密钥策略：strict | accept-new | insecure（不推荐）。")
	setMappingHeadComment(defaults, "batch", "批量操作的默认并发数和连接超时。")
	setMappingHeadComment(defaults, "exec", "远程命令的默认执行超时。")
	setMappingHeadComment(defaults, "transfer", "push/pull 文件传输的默认超时。")
	setMappingHeadComment(defaults, "logs", "操作日志开关和保留时间。")

	batch := mappingValue(defaults, "batch")
	setMappingHeadComment(batch, "parallel", "最大并发主机数，允许 1-128。")
	setMappingHeadComment(batch, "connect_timeout", "单台主机建立 SSH 连接的最长等待时间。")
	setMappingHeadComment(mappingValue(defaults, "exec"), "timeout", "单条远程命令的最长执行时间。")
	setMappingHeadComment(mappingValue(defaults, "transfer"), "timeout", "单次文件传输的最长执行时间。")
	logs := mappingValue(defaults, "logs")
	setMappingHeadComment(logs, "enabled", "是否记录操作日志。")
	setMappingHeadComment(logs, "retention", "日志保留时间，例如 30d、12h。")

	setMappingHeadComment(mappingValue(document, "tags"), "items", "标签名称及可选说明。")
	setMappingHeadComment(mappingValue(document, "managed_keys"), "items", "托管密钥公钥信息；私钥加密保存在 vault 中。")
	setMappingHeadComment(mappingValue(document, "host_trust"), "entries", "每个地址和端口对应的已确认 SSH 公钥。")
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func setMappingHeadComment(mapping *yaml.Node, key, comment string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index].HeadComment = comment
			return
		}
	}
}

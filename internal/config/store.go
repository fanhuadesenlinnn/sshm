package config

import (
	"fmt"
	"os"

	"github.com/fanhuadesenlinnn/sshm/internal/safefile"
	"gopkg.in/yaml.v3"
)

const CurrentVersion = 2

// Store manages host configuration persistence.
type Store struct {
	path string
}

// NewStore creates a new Store using the default hosts.yaml path.
func NewStore() *Store {
	return &Store{path: HostsFilePath()}
}

// NewStoreWithPath creates a Store with an explicit file path.
func NewStoreWithPath(path string) *Store {
	return &Store{path: path}
}

// Path returns the store file path.
func (s *Store) Path() string { return s.path }

// Load reads and parses hosts.yaml.  Missing IDs are filled automatically.
// v1 configs (without IDs) are migrated transparently.
func (s *Store) Load() (*HostsFile, error) {
	var hf *HostsFile
	err := safefile.WithLock(s.path, func() error {
		var err error
		hf, err = s.loadUnlocked(true)
		return err
	})
	return hf, err
}

func (s *Store) loadUnlocked(persistMigration bool) (*HostsFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HostsFile{Version: CurrentVersion, Hosts: []Host{}}, nil
		}
		return nil, fmt.Errorf("读取配置文件失败（可检查备份 %s.bak）: %w", s.path, err)
	}
	var hf HostsFile
	if err := yaml.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("解析配置文件失败（主文件未修改，可检查备份 %s.bak）: %w", s.path, err)
	}
	if hf.Hosts == nil {
		hf.Hosts = []Host{}
	}
	changed := hf.EnsureIDs()
	if hf.Version < CurrentVersion {
		hf.Version = CurrentVersion
		changed = true
	}
	if changed && persistMigration {
		if err := s.saveUnlocked(&hf); err != nil {
			return nil, fmt.Errorf("迁移配置到 v%d 失败，原文件已保留: %w", CurrentVersion, err)
		}
	}
	return &hf, nil
}

// Save writes hosts.yaml with a backup and atomic rename.
func (s *Store) Save(hf *HostsFile) error {
	return safefile.WithLock(s.path, func() error {
		return s.saveUnlocked(hf)
	})
}

func (s *Store) saveUnlocked(hf *HostsFile) error {
	hf.Version = CurrentVersion
	hf.EnsureIDs()
	if dups := hf.DuplicateAliases(); len(dups) > 0 {
		return fmt.Errorf("配置包含重复别名: %v", dups)
	}
	data, err := yaml.Marshal(hf)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	return safefile.Write(s.path, data, 0600, true)
}

// FindByAlias searches for a host by alias.
func (s *Store) FindByAlias(alias string) (*Host, int, error) {
	hf, err := s.Load()
	if err != nil {
		return nil, -1, err
	}
	for i, h := range hf.Hosts {
		if h.Alias == alias {
			return &hf.Hosts[i], i, nil
		}
	}
	return nil, -1, nil
}

// FindByID searches for a host by numeric ID (1-based index in the list).
func (s *Store) FindByID(id int) (*Host, int, error) {
	hf, err := s.Load()
	if err != nil {
		return nil, -1, err
	}
	if id < 1 || id > len(hf.Hosts) {
		return nil, -1, fmt.Errorf("ID %d 超出范围 (1-%d)", id, len(hf.Hosts))
	}
	return &hf.Hosts[id-1], id - 1, nil
}

// FindHost resolves an alias or ID string to a host entry.
func (s *Store) FindHost(aliasOrID string) (*Host, int, *HostsFile, error) {
	hf, err := s.Load()
	if err != nil {
		return nil, -1, nil, err
	}

	// Try numeric ID first
	if id := parseID(aliasOrID); id > 0 {
		if id > len(hf.Hosts) {
			return nil, -1, nil, fmt.Errorf("ID %d 超出范围 (1-%d)", id, len(hf.Hosts))
		}
		return &hf.Hosts[id-1], id - 1, hf, nil
	}

	// Try alias
	for i, h := range hf.Hosts {
		if h.Alias == aliasOrID {
			return &hf.Hosts[i], i, hf, nil
		}
	}

	return nil, -1, nil, fmt.Errorf("未找到主机: %s", aliasOrID)
}

func parseID(s string) int {
	if s == "" {
		return 0
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
	}
	var n int
	for _, ch := range s {
		n = n*10 + int(ch-'0')
	}
	return n
}

// Add appends a host to the store after checking for duplicate aliases.
func (s *Store) Add(h Host) error {
	return safefile.WithLock(s.path, func() error {
		hf, err := s.loadUnlocked(false)
		if err != nil {
			return err
		}
		for _, existing := range hf.Hosts {
			if existing.Alias == h.Alias {
				return fmt.Errorf("别名 '%s' 已存在", h.Alias)
			}
		}
		if h.ID == "" {
			h.ID = NewID()
		}
		hf.Hosts = append(hf.Hosts, h)
		return s.saveUnlocked(hf)
	})
}

// Update replaces a host at the given index after duplicate check.
func (s *Store) Update(idx int, h Host) error {
	return safefile.WithLock(s.path, func() error {
		hf, err := s.loadUnlocked(false)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= len(hf.Hosts) {
			return fmt.Errorf("索引 %d 超出范围", idx)
		}
		for i, existing := range hf.Hosts {
			if i != idx && existing.Alias == h.Alias {
				return fmt.Errorf("别名 '%s' 已存在", h.Alias)
			}
		}
		hf.Hosts[idx] = h
		return s.saveUnlocked(hf)
	})
}

// Remove deletes a host at the given index.
func (s *Store) Remove(idx int) error {
	return safefile.WithLock(s.path, func() error {
		hf, err := s.loadUnlocked(false)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= len(hf.Hosts) {
			return fmt.Errorf("索引 %d 超出范围", idx)
		}
		hf.Hosts = append(hf.Hosts[:idx], hf.Hosts[idx+1:]...)
		return s.saveUnlocked(hf)
	})
}

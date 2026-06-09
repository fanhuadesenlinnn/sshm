package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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

// Load reads and parses hosts.yaml.
func (s *Store) Load() (*HostsFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HostsFile{Version: 1, Hosts: []Host{}}, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var hf HostsFile
	if err := yaml.Unmarshal(data, &hf); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if hf.Version == 0 {
		hf.Version = 1
	}
	if hf.Hosts == nil {
		hf.Hosts = []Host{}
	}
	return &hf, nil
}

// Save writes hosts.yaml with proper permissions.
func (s *Store) Save(hf *HostsFile) error {
	if hf.Version == 0 {
		hf.Version = 1
	}
	data, err := yaml.Marshal(hf)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
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

// Add appends a host to the store.
func (s *Store) Add(h Host) error {
	hf, err := s.Load()
	if err != nil {
		return err
	}
	hf.Hosts = append(hf.Hosts, h)
	return s.Save(hf)
}

// Update replaces a host at the given index.
func (s *Store) Update(idx int, h Host) error {
	hf, err := s.Load()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(hf.Hosts) {
		return fmt.Errorf("索引 %d 超出范围", idx)
	}
	hf.Hosts[idx] = h
	return s.Save(hf)
}

// Remove deletes a host at the given index.
func (s *Store) Remove(idx int) error {
	hf, err := s.Load()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(hf.Hosts) {
		return fmt.Errorf("索引 %d 超出范围", idx)
	}
	hf.Hosts = append(hf.Hosts[:idx], hf.Hosts[idx+1:]...)
	return s.Save(hf)
}

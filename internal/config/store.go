package config

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = DocumentVersion

// Store is a host-focused view over the single sshm.yaml repository.
type Store struct {
	repo *Repository
}

func NewStore() *Store {
	return &Store{repo: NewRepository()}
}

func NewStoreWithPath(path string) *Store {
	return &Store{repo: NewRepositoryWithPath(path)}
}

func (s *Store) Path() string { return s.repo.Path() }

func (s *Store) Repository() *Repository { return s.repo }

func (s *Store) Load() (*HostsFile, error) {
	doc, err := s.repo.Load()
	if err != nil {
		return nil, err
	}
	hosts := append([]Host(nil), doc.Hosts...)
	for i := range hosts {
		hosts[i].ConfigPath = s.repo.Path()
		hosts[i].ResolvedHostKeyPolicy = hosts[i].EffectiveHostKeyPolicy(doc.Defaults)
	}
	return &HostsFile{Version: CurrentVersion, Hosts: hosts}, nil
}

func (s *Store) Save(hf *HostsFile) error {
	hosts := append([]Host(nil), hf.Hosts...)
	return s.repo.Update(func(doc *Document) error {
		doc.Hosts = hosts
		return nil
	})
}

func ValidateHostsData(data []byte) (*HostsFile, error) {
	var hf HostsFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&hf); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if hf.Hosts == nil {
		hf.Hosts = []Host{}
	}
	seenAlias := map[string]bool{}
	seenID := map[string]bool{}
	for i := range hf.Hosts {
		hf.Hosts[i].EnsureDefaults()
		if hf.Hosts[i].ID == "" {
			hf.Hosts[i].ID = NewID()
		}
		if seenAlias[hf.Hosts[i].Alias] {
			return nil, fmt.Errorf("配置包含重复别名: %s", hf.Hosts[i].Alias)
		}
		if seenID[hf.Hosts[i].ID] {
			return nil, fmt.Errorf("配置包含重复稳定 ID: %s", hf.Hosts[i].ID)
		}
		seenAlias[hf.Hosts[i].Alias] = true
		seenID[hf.Hosts[i].ID] = true
		if errs := hf.Hosts[i].Validate(); len(errs) > 0 {
			return nil, fmt.Errorf("主机 %s 配置无效: %v", hf.Hosts[i].Alias, errs)
		}
	}
	hf.Version = CurrentVersion
	return &hf, nil
}

func (s *Store) FindByAlias(alias string) (*Host, int, error) {
	hf, err := s.Load()
	if err != nil {
		return nil, -1, err
	}
	for i := range hf.Hosts {
		if hf.Hosts[i].Alias == alias {
			return &hf.Hosts[i], i, nil
		}
	}
	return nil, -1, nil
}

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

func (s *Store) FindHost(aliasOrID string) (*Host, int, *HostsFile, error) {
	hf, err := s.Load()
	if err != nil {
		return nil, -1, nil, err
	}
	if id := parseID(aliasOrID); id > 0 {
		if id > len(hf.Hosts) {
			return nil, -1, nil, fmt.Errorf("ID %d 超出范围 (1-%d)", id, len(hf.Hosts))
		}
		return &hf.Hosts[id-1], id - 1, hf, nil
	}
	for i := range hf.Hosts {
		if hf.Hosts[i].Alias == aliasOrID {
			return &hf.Hosts[i], i, hf, nil
		}
	}
	return nil, -1, nil, fmt.Errorf("未找到主机: %s", aliasOrID)
}

func parseID(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func (s *Store) Add(h Host) error {
	return s.repo.Update(func(doc *Document) error {
		for _, existing := range doc.Hosts {
			if existing.Alias == h.Alias {
				return fmt.Errorf("别名 '%s' 已存在", h.Alias)
			}
		}
		if h.ID == "" {
			h.ID = NewID()
		}
		h.EnsureDefaults()
		doc.Hosts = append(doc.Hosts, h)
		return nil
	})
}

func (s *Store) Update(idx int, h Host) error {
	return s.repo.Update(func(doc *Document) error {
		if idx < 0 || idx >= len(doc.Hosts) {
			return fmt.Errorf("索引 %d 超出范围", idx)
		}
		for i, existing := range doc.Hosts {
			if i != idx && existing.Alias == h.Alias {
				return fmt.Errorf("别名 '%s' 已存在", h.Alias)
			}
		}
		h.EnsureDefaults()
		doc.Hosts[idx] = h
		return nil
	})
}

func (s *Store) Remove(idx int) error {
	return s.repo.Update(func(doc *Document) error {
		if idx < 0 || idx >= len(doc.Hosts) {
			return fmt.Errorf("索引 %d 超出范围", idx)
		}
		doc.Hosts = append(doc.Hosts[:idx], doc.Hosts[idx+1:]...)
		return nil
	})
}

func (s *Store) MarkUsed(id, timestamp string) error {
	return s.repo.Update(func(doc *Document) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID == id {
				doc.Hosts[i].LastUsedAt = timestamp
				return nil
			}
		}
		return fmt.Errorf("未找到主机 ID: %s", id)
	})
}

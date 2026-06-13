package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

// Host represents a single SSH host entry.
type Host struct {
	ID          string   `yaml:"id"` // stable UUID, auto-generated if empty
	Alias       string   `yaml:"alias"`
	User        string   `yaml:"user"`
	Host        string   `yaml:"host"`
	Port        int      `yaml:"port"`
	Identity    string   `yaml:"identity"`
	Note        string   `yaml:"note"`
	Group       string   `yaml:"group"`
	Tags        []string `yaml:"tags"`
	Auth        string   `yaml:"auth"`
	PasswordRef string   `yaml:"password_ref"`
	Pinned      bool     `yaml:"pinned,omitempty"`
	LastUsedAt  string   `yaml:"last_used_at,omitempty"`
}

// HostsFile is the top-level structure of hosts.yaml.
type HostsFile struct {
	Version int    `yaml:"version"`
	Hosts   []Host `yaml:"hosts"`
}

var aliasRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var fallbackIDCounter uint64

// NewID generates a stable 128-bit identifier.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		seed := uint64(time.Now().UnixNano()) ^ atomic.AddUint64(&fallbackIDCounter, 1)
		for i := range b {
			b[i] = byte(seed >> ((i % 8) * 8))
		}
	}
	return hex.EncodeToString(b)
}

// EnsureIDs fills missing IDs for all hosts and returns whether any were added.
func (hf *HostsFile) EnsureIDs() bool {
	changed := false
	for i := range hf.Hosts {
		if hf.Hosts[i].ID == "" {
			hf.Hosts[i].ID = NewID()
			changed = true
		}
	}
	return changed
}

// EnsureDefaults fills fields omitted by older or hand-written configs.
func (hf *HostsFile) EnsureDefaults() bool {
	changed := false
	for i := range hf.Hosts {
		if hf.Hosts[i].Port == 0 {
			hf.Hosts[i].Port = 22
			changed = true
		}
		if hf.Hosts[i].Auth == "" {
			hf.Hosts[i].Auth = "auto"
			changed = true
		}
		if hf.Hosts[i].Tags == nil {
			hf.Hosts[i].Tags = []string{}
			changed = true
		}
	}
	return changed
}

// DuplicateAliases returns a list of duplicate alias strings.
func (hf *HostsFile) DuplicateAliases() []string {
	seen := map[string]int{}
	for _, h := range hf.Hosts {
		seen[h.Alias]++
	}
	var dups []string
	for alias, count := range seen {
		if count > 1 {
			dups = append(dups, alias)
		}
	}
	return dups
}

// Validate checks the host entry and returns a list of errors.
func (h *Host) Validate() []string {
	var errs []string

	if h.Alias == "" {
		errs = append(errs, "别名不能为空")
	} else if !aliasRegexp.MatchString(h.Alias) {
		errs = append(errs, "别名只能包含字母、数字、点、下划线、短横线")
	}
	// Check alias is not purely numeric
	if h.Alias != "" {
		if _, err := strconv.Atoi(h.Alias); err == nil {
			errs = append(errs, "别名不能是纯数字")
		}
	}

	if h.User == "" {
		errs = append(errs, "用户名不能为空")
	}
	if h.Host == "" {
		errs = append(errs, "主机/IP 不能为空")
	}
	if h.Port < 1 || h.Port > 65535 {
		errs = append(errs, fmt.Sprintf("端口必须在 1-65535 之间，当前: %d", h.Port))
	}

	validAuth := map[string]bool{
		"auto": true, "key": true, "password": true, "ask": true, "system": true,
	}
	if !validAuth[h.Auth] {
		errs = append(errs, fmt.Sprintf("无效的认证策略: %s", h.Auth))
	}

	return errs
}

// DefaultHost returns a Host with default values.
func DefaultHost() Host {
	return Host{
		ID:   NewID(),
		Port: 22,
		Auth: "auto",
		Tags: []string{},
	}
}

// ParseTags converts a comma-separated or space-separated string to a tag slice.
func ParseTags(input string) []string {
	if input == "" {
		return []string{}
	}
	var result []string
	seen := map[string]bool{}
	for _, part := range splitBy(input, ',') {
		for _, sub := range splitBy(part, ' ') {
			if sub == "" {
				continue
			}
			if !seen[sub] {
				seen[sub] = true
				result = append(result, sub)
			}
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

// TagsToString joins tags into a space-separated string.
func TagsToString(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	result := ""
	for i, t := range tags {
		if i > 0 {
			result += " "
		}
		result += t
	}
	return result
}

func splitBy(s string, sep rune) []string {
	var parts []string
	current := ""
	for _, ch := range s {
		if ch == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

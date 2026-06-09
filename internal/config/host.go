package config

import (
	"fmt"
	"regexp"
	"strconv"
)

// Host represents a single SSH host entry.
type Host struct {
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
}

// HostsFile is the top-level structure of hosts.yaml.
type HostsFile struct {
	Version int    `yaml:"version"`
	Hosts   []Host `yaml:"hosts"`
}

var aliasRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

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
	// Split by commas first, then by spaces
	var result []string
	// Try comma-separated first
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

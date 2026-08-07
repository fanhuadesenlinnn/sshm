// Package deploy implements the Deploy v3 engine: shared host-selection and
// condition types, plan building, modules and the batch runner integration.
package deploy

import "strings"

// TargetSelector picks the hosts a play applies to. Exactly one of hosts,
// tags or all should be used.
type TargetSelector struct {
	Hosts       []string `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	All         bool     `yaml:"all,omitempty" json:"all,omitempty"`
	Exclude     []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	ExcludeTags []string `yaml:"exclude_tags,omitempty" json:"exclude_tags,omitempty"`
}

// Empty reports whether no selector is configured.
func (s TargetSelector) Empty() bool {
	return !s.All && len(s.Hosts) == 0 && len(s.Tags) == 0
}

// String renders a human-readable selector summary.
func (s TargetSelector) String() string {
	if s.All {
		return "all"
	}
	value := ""
	if len(s.Hosts) > 0 {
		value = "host:" + strings.Join(s.Hosts, ",")
	}
	if len(s.Tags) > 0 {
		if value != "" {
			value += " + "
		}
		value += "tag:" + strings.Join(s.Tags, ",")
	}
	return value
}

package config

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tag describes a named host classification stored in sshm.yaml.
type Tag struct {
	Name string `yaml:"name"`
	Note string `yaml:"note,omitempty"`
}

// TagsFile is the tag section of sshm.yaml.
type TagsFile struct {
	Items []Tag `yaml:"items"`
}

// ValidateTagName checks names used by tag commands and host references.
func ValidateTagName(name string) error {
	if name == "" {
		return fmt.Errorf("标签名称不能为空")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("标签名称不能以短横线开头")
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("标签名称不能超过 64 个字符")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == ',' {
			return fmt.Errorf("标签名称不能包含空白、逗号或控制字符")
		}
	}
	return nil
}

func (tf *TagsFile) normalize() {
	if tf.Items == nil {
		tf.Items = []Tag{}
	}
	sort.SliceStable(tf.Items, func(i, j int) bool {
		return tf.Items[i].Name < tf.Items[j].Name
	})
}

// Find returns a tag definition by name.
func (tf *TagsFile) Find(name string) (*Tag, bool) {
	for i := range tf.Items {
		if tf.Items[i].Name == name {
			return &tf.Items[i], true
		}
	}
	return nil, false
}

// Ensure creates missing tag definitions.
func (tf *TagsFile) Ensure(names ...string) {
	known := make(map[string]bool, len(tf.Items))
	for _, tag := range tf.Items {
		known[tag.Name] = true
	}
	for _, name := range names {
		if !known[name] {
			tf.Items = append(tf.Items, Tag{Name: name})
			known[name] = true
		}
	}
	tf.normalize()
}

// NormalizeTags trims and deduplicates host tag references.
func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
	}
	return result
}

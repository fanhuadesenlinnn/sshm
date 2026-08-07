package deploy

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/shellquote"
)

// MergeVars overlays layers in order; later layers win.
func MergeVars(layers ...Vars) Vars {
	out := Vars{}
	for _, layer := range layers {
		for key, value := range layer {
			out[key] = value
		}
	}
	return out
}

func cloneVars(source Vars) Vars {
	out := Vars{}
	for key, value := range source {
		out[key] = value
	}
	return out
}

// RenderString interpolates {{ }} expressions using vars. Missing variables
// are hard errors unless accessed through the var() helper with a default.
func RenderString(value string, vars Vars) (string, error) {
	value = preprocessTemplate(value)
	tmpl, err := template.New("vars").
		Option("missingkey=error").
		Funcs(templateFuncs(vars)).
		Parse(value)
	if err != nil {
		return "", fmt.Errorf("模板语法错误 %q: %w", value, err)
	}
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, vars); err != nil {
		return "", fmt.Errorf("模板渲染失败 %q: %w", value, err)
	}
	return buffer.String(), nil
}

var templateBlockPattern = regexp.MustCompile(`\{\{(-)?\s*([^}]+?)\s*(-)?\}\}`)

var templateFunctions = map[string]bool{
	"var": true, "default": true, "join": true, "upper": true, "lower": true,
	"trim": true, "replace": true, "shellquote": true,
}

// preprocessTemplate converts Ansible-style {{ name }} into Go template
// {{ .name }} for plain identifiers while leaving function calls untouched.
func preprocessTemplate(value string) string {
	return templateBlockPattern.ReplaceAllStringFunc(value, func(block string) string {
		matches := templateBlockPattern.FindStringSubmatch(block)
		if len(matches) < 3 {
			return block
		}
		inner := strings.TrimSpace(matches[2])
		leftDash := matches[1]
		rightDash := matches[3]
		pipeline := strings.Split(inner, "|")
		head := strings.TrimSpace(pipeline[0])
		if head != "" && !strings.HasPrefix(head, ".") && isPlainPath(head) && !templateFunctions[firstWord(head)] {
			if hasDefaultFilter(pipeline) {
				pipeline[0] = `var "` + head + `"`
			} else {
				pipeline[0] = "." + head
			}
		}
		rebuilt := "{{"
		if leftDash != "" {
			rebuilt += "-"
		}
		rebuilt += " " + strings.Join(pipeline, " | ") + " "
		if rightDash != "" {
			rebuilt += "-"
		}
		rebuilt += "}}"
		return rebuilt
	})
}

func hasDefaultFilter(pipeline []string) bool {
	for _, segment := range pipeline[1:] {
		if strings.HasPrefix(strings.TrimSpace(segment), "default") {
			return true
		}
	}
	return false
}

func isPlainPath(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

func firstWord(value string) string {
	if index := strings.IndexByte(value, '.'); index >= 0 {
		return value[:index]
	}
	return value
}

func templateFuncs(vars Vars) template.FuncMap {
	return template.FuncMap{
		"var": func(name string) any {
			parts := strings.Split(name, ".")
			var current any = any(vars)
			for _, part := range parts {
				value, ok := mapLookup(current, part)
				if !ok {
					return nil
				}
				current = value
			}
			return current
		},
		"default": func(def, value any) any {
			if isEmptyValue(value) {
				return def
			}
			return value
		},
		"join": func(separator string, values []any) string {
			parts := make([]string, 0, len(values))
			for _, value := range values {
				parts = append(parts, fmt.Sprintf("%v", value))
			}
			return strings.Join(parts, separator)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
		"replace": func(old, new, value string) string {
			return strings.ReplaceAll(value, old, new)
		},
		"shellquote": shellquote.Single,
	}
}

func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return reflected.Len() == 0
	}
	return false
}

func mapLookup(value any, key string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		item, ok := typed[key]
		return item, ok
	case Vars:
		item, ok := typed[key]
		return item, ok
	case map[string]string:
		item, ok := typed[key]
		return item, ok
	default:
		return nil, false
	}
}

// renderNode mutates scalar string values inside a YAML node in place.
func renderNode(node *yaml.Node, vars Vars) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := renderNode(child, vars); err != nil {
				return err
			}
		}
		return nil
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return nil
		}
		rendered, err := RenderString(node.Value, vars)
		if err != nil {
			return err
		}
		node.Value = rendered
		return nil
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			if err := renderNode(node.Content[index+1], vars); err != nil {
				return err
			}
		}
		return nil
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := renderNode(child, vars); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// renderArgs returns a copy of the argument node with strings interpolated.
func renderArgs(node *yaml.Node, vars Vars) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	copy := deepCopyNode(node)
	if err := renderNode(copy, vars); err != nil {
		return nil, err
	}
	return copy, nil
}

func deepCopyNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	copy := *node
	copy.Content = make([]*yaml.Node, len(node.Content))
	for index, child := range node.Content {
		copy.Content[index] = deepCopyNode(child)
	}
	copy.Alias = nil
	return &copy
}

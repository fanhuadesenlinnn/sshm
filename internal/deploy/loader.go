package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"gopkg.in/yaml.v3"
)

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func Discover(explicit []string, _ string) ([]string, error) {
	if len(explicit) > 0 {
		paths := make([]string, 0, len(explicit))
		for _, item := range explicit {
			absolute, err := filepath.Abs(config.ExpandPath(item))
			if err != nil {
				return nil, fmt.Errorf("解析 deploy 文件 %s 失败: %w", item, err)
			}
			paths = append(paths, absolute)
		}
		return unique(paths), nil
	}
	var paths []string
	if exists(config.DeployFilePath()) {
		paths = append(paths, config.DeployFilePath())
	}
	fragments, err := filepath.Glob(filepath.Join(config.DeployDir(), "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(fragments)
	paths = append(paths, fragments...)
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到 deploy 配置；使用 sshm deploy init 或通过 --file 指定文件")
	}
	return paths, nil
}

func Load(paths []string) (*Catalog, error) {
	catalog := &Catalog{
		Sources: append([]string(nil), paths...),
		ByName:  map[string]Profile{}, HandlerByName: map[string]Step{},
	}
	for _, path := range paths {
		file, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		for _, profile := range file.Profiles {
			profile.Source = file.Path
			profile.BaseDir = file.BaseDir
			for index := range profile.Steps {
				profile.Steps[index].BaseDir = file.BaseDir
			}
			if previous, exists := catalog.ByName[profile.Name]; exists {
				return nil, fmt.Errorf("profile %q 在 %s 与 %s 中重复", profile.Name, previous.Source, profile.Source)
			}
			catalog.Profiles = append(catalog.Profiles, profile)
			catalog.ByName[profile.Name] = profile
		}
		for _, handler := range file.Handlers {
			handler.BaseDir = file.BaseDir
			if previous, exists := catalog.HandlerByName[handler.Name]; exists {
				return nil, fmt.Errorf("handler %q 在多个文件中重复（此前 action=%s）", handler.Name, previous.ActionType())
			}
			catalog.Handlers = append(catalog.Handlers, handler)
			catalog.HandlerByName[handler.Name] = handler
		}
	}
	return catalog, nil
}

func loadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 deploy 配置 %s 失败: %w", path, err)
	}
	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("解析 deploy 配置 %s 失败: %w", path, err)
	}
	if file.Version == 0 {
		return nil, fmt.Errorf("%s 缺少必填字段 version", path)
	}
	if file.Version != Version {
		return nil, fmt.Errorf("%s 使用不支持的 deploy 配置版本 %d，当前仅支持 %d", path, file.Version, Version)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	file.Path = absolute
	file.BaseDir = filepath.Dir(absolute)
	return &file, nil
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

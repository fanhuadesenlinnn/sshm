package deploy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"gopkg.in/yaml.v3"
)

var variablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func Discover(explicit []string, cwd string) ([]string, error) {
	if len(explicit) > 0 {
		paths := make([]string, 0, len(explicit))
		for _, path := range explicit {
			absolute, err := filepath.Abs(config.ExpandPath(path))
			if err != nil {
				return nil, err
			}
			paths = append(paths, absolute)
		}
		return unique(paths), nil
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	var paths []string
	global := filepath.Join(config.SSHMHome(), "deploy.yaml")
	if exists(global) {
		paths = append(paths, global)
	}
	fragments, err := filepath.Glob(filepath.Join(config.SSHMHome(), "deploy.d", "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(fragments)
	paths = append(paths, fragments...)
	local := filepath.Join(cwd, "sshm.deploy.yaml")
	if exists(local) {
		paths = append(paths, local)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到 deploy 配置；使用 sshm deploy init 或通过 -f 指定文件")
	}
	return unique(paths), nil
}

func Load(paths []string) (*Catalog, error) {
	catalog := &Catalog{Sources: append([]string(nil), paths...), ByName: map[string]Profile{}}
	for _, path := range paths {
		file, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		for _, profile := range file.Profiles {
			profile.Source = file.Path
			profile.BaseDir = file.BaseDir
			profile.Vars = cloneVars(file.Vars)
			profile.Strategy = mergeStrategy(file.Defaults.Strategy, profile.Strategy)
			if previous, exists := catalog.ByName[profile.Name]; exists {
				return nil, fmt.Errorf("profile %q 在 %s 与 %s 中重复", profile.Name, previous.Source, profile.Source)
			}
			catalog.Profiles = append(catalog.Profiles, profile)
			catalog.ByName[profile.Name] = profile
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
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	file.Path = absolute
	file.BaseDir = filepath.Dir(absolute)
	if file.Version == 0 {
		file.Version = Version
	}
	if file.Version != Version {
		return nil, fmt.Errorf("%s 使用不支持的 deploy 配置版本 %d", path, file.Version)
	}
	if file.Vars == nil {
		file.Vars = map[string]string{}
	}
	return &file, nil
}

func mergeStrategy(defaults, override Strategy) Strategy {
	result := defaults
	if override.Mode != "" {
		result.Mode = override.Mode
		if override.Mode == "visible" && override.MaxParallel == 0 {
			result.MaxParallel = 1
		}
	}
	if override.maxParallelSet || override.MaxParallel != 0 {
		result.MaxParallel = override.MaxParallel
		result.maxParallelSet = override.maxParallelSet
	}
	if override.ConnectTimeout.Duration != 0 {
		result.ConnectTimeout = override.ConnectTimeout
	}
	if override.StepTimeout.Duration != 0 {
		result.StepTimeout = override.StepTimeout
	}
	if override.RunTimeout.Duration != 0 {
		result.RunTimeout = override.RunTimeout
	}
	if override.retryCountSet {
		result.RetryCount = override.RetryCount
		result.retryCountSet = true
	}
	if override.RetryOnStage != nil {
		result.RetryOnStage = append([]string(nil), override.RetryOnStage...)
	}
	return applyStrategyDefaults(result)
}

func applyStrategyDefaults(strategy Strategy) Strategy {
	if strategy.Mode == "" {
		strategy.Mode = "hidden"
	}
	if strategy.MaxParallel == 0 && !strategy.maxParallelSet {
		strategy.MaxParallel = 5
		if strategy.Mode == "visible" {
			strategy.MaxParallel = 1
		}
	}
	if strategy.ConnectTimeout.Duration == 0 {
		strategy.ConnectTimeout.Duration = 30 * timeSecond
	}
	if strategy.StepTimeout.Duration == 0 {
		strategy.StepTimeout.Duration = 15 * timeMinute
	}
	if strategy.RetryOnStage == nil {
		strategy.RetryOnStage = []string{"network", "transfer"}
	}
	return strategy
}

const (
	timeSecond = 1_000_000_000
	timeMinute = 60 * timeSecond
)

func Expand(input string, vars map[string]string) (string, error) {
	var missing []string
	output := variablePattern.ReplaceAllStringFunc(input, func(token string) string {
		name := variablePattern.FindStringSubmatch(token)[1]
		value, ok := vars[name]
		if !ok {
			missing = append(missing, name)
			return token
		}
		return value
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("未定义变量: %s", strings.Join(unique(missing), ", "))
	}
	return output, nil
}

func cloneVars(vars map[string]string) map[string]string {
	cloned := make(map[string]string, len(vars))
	for key, value := range vars {
		cloned[key] = value
	}
	return cloned
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

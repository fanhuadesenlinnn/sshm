package deployv3

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
)

const maxIncludeDepth = 10

// Overrides carries command-line adjustments applied on top of a play.
type Overrides struct {
	Targets               *deploy.TargetSelector
	Parallel              int
	Serial                int
	FailFast              bool
	MaxFail               int
	MaxFailPercent        int
	Check                 bool
	Diff                  bool
	ExtraVars             Vars
	DefaultParallel       int
	DefaultTimeout        config.Duration
	DefaultConnectTimeout config.Duration
}

// Plan is a fully resolved version-3 play ready to execute.
type Plan struct {
	Name           string
	Strategy       string
	Config         string
	Description    string
	Sources        []string
	Selector       deploy.TargetSelector
	Hosts          []config.Host
	Batch          batch.Options
	Timeout        config.Duration
	ConnectTimeout config.Duration
	Check          bool
	Diff           bool
	Vars           Vars
	Tasks          []Task
	GatherFacts    bool
	FactsDir       string
}

// Discover returns deploy file paths (shared with the v2 loader).
func Discover(explicit []string) ([]string, error) {
	return deploy.Discover(explicit, "")
}

// Load reads and merges version-3 files into a catalog. Includes and
// vars_files are resolved later by BuildPlan.
func Load(paths []string) (*Catalog, error) {
	catalog := &Catalog{
		Sources: append([]string(nil), paths...),
		ByName:  map[string]Play{},
	}
	for _, path := range paths {
		file, err := loadFile(path)
		if err != nil {
			return nil, err
		}
		catalog.Files = append(catalog.Files, file)
		for _, play := range file.Plays {
			play.Source = file.Path
			play.BaseDir = file.BaseDir
			if previous, exists := catalog.ByName[play.Name]; exists {
				return nil, fmt.Errorf("play %q 在 %s 与 %s 中重复", play.Name, previous.Source, file.Path)
			}
			catalog.ByName[play.Name] = play
			catalog.Plays = append(catalog.Plays, play)
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

// BuildPlan resolves a named play into an executable plan.
func BuildPlan(catalog *Catalog, name string, hosts []config.Host, overrides Overrides) (*Plan, error) {
	play, ok := catalog.ByName[name]
	if !ok {
		return nil, fmt.Errorf("未找到 deploy play: %s", name)
	}
	selector := play.Hosts
	if overrides.Targets != nil {
		selector = *overrides.Targets
	}
	if err := validatePlayShape(play); err != nil {
		return nil, err
	}
	resolvedHosts, err := deploy.ResolveTargets(hosts, selector)
	if err != nil {
		return nil, err
	}
	tasks, err := expandIncludes(play.Tasks, play.BaseDir, nil, 0)
	if err != nil {
		return nil, err
	}
	vars, err := resolveVars(play, catalog, overrides)
	if err != nil {
		return nil, err
	}
	validationVars := withFactPlaceholders(vars)
	for index, task := range tasks {
		if err := validateTask(task, validationVars); err != nil {
			return nil, fmt.Errorf("%s: play %q task %d (%s): %w", play.Source, play.Name, index+1, task.DisplayName(index), err)
		}
	}
	parallel := play.Parallel
	if parallel == 0 {
		parallel = overrides.DefaultParallel
	}
	if parallel == 0 {
		parallel = 4
	}
	if overrides.Parallel > 0 {
		parallel = overrides.Parallel
	}
	serial := play.Serial
	if overrides.Serial > 0 {
		serial = overrides.Serial
	}
	timeout := play.Timeout
	if timeout.Duration == 0 {
		timeout = overrides.DefaultTimeout
	}
	if timeout.Duration == 0 {
		timeout.Duration = 30 * time.Second
	}
	connectTimeout := play.ConnectTimeout
	if connectTimeout.Duration == 0 {
		connectTimeout = overrides.DefaultConnectTimeout
	}
	if connectTimeout.Duration == 0 {
		connectTimeout.Duration = 10 * time.Second
	}
	batchOptions := batch.Options{
		Parallel:       parallel,
		Serial:         serial,
		FailFast:       play.FailFast || overrides.FailFast,
		MaxFail:        play.MaxFail,
		MaxFailPercent: play.MaxFailPercent,
	}
	if overrides.MaxFail > 0 {
		batchOptions.MaxFail = overrides.MaxFail
	}
	if overrides.MaxFailPercent > 0 {
		batchOptions.MaxFailPercent = overrides.MaxFailPercent
	}
	plan := &Plan{
		Name: name, Strategy: play.StrategyOrDefault(), Config: play.Source, Description: play.Description,
		Sources: append([]string(nil), catalog.Sources...), Selector: selector,
		Hosts: resolvedHosts, Batch: batchOptions, Timeout: timeout,
		ConnectTimeout: connectTimeout, Check: overrides.Check, Diff: overrides.Diff,
		Vars: vars, Tasks: tasks, GatherFacts: play.GatherFacts,
		FactsDir: filepath.Join(config.SSHMHome(), "facts"),
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

var factPlaceholders = []string{"hostname", "system", "arch", "os_family", "os_id"}

// withFactPlaceholders injects empty values for runtime-only facts so plan-time
// argument rendering validates playbooks that reference facts.
func withFactPlaceholders(vars Vars) Vars {
	out := cloneVars(vars)
	for _, key := range factPlaceholders {
		if _, ok := out[key]; !ok {
			out[key] = ""
		}
		if _, ok := out["ansible_"+key]; !ok {
			out["ansible_"+key] = ""
		}
	}
	return out
}

func (p *Plan) Validate() error {
	if err := p.Batch.Validate(); err != nil {
		return err
	}
	if p.Timeout.Duration <= 0 || p.ConnectTimeout.Duration <= 0 {
		return fmt.Errorf("deploy timeout 和 connect_timeout 必须大于 0")
	}
	return nil
}

func validatePlayShape(play Play) error {
	if strings.TrimSpace(play.Name) == "" {
		return fmt.Errorf("play 名称不能为空")
	}
	if play.Hosts.Empty() {
		return fmt.Errorf("play %q 的 hosts 不能为空", play.Name)
	}
	if len(play.Tasks) == 0 {
		return fmt.Errorf("play %q 至少需要一个 task", play.Name)
	}
	strategy := play.StrategyOrDefault()
	if strategy != StrategyLinear && strategy != StrategyFree {
		return fmt.Errorf("play %q 的 strategy 必须是 linear 或 free", play.Name)
	}
	if play.Serial < 0 || play.Parallel < 0 || play.Parallel > 128 {
		return fmt.Errorf("play %q 的 serial/parallel 参数非法", play.Name)
	}
	if play.MaxFail < 0 || play.MaxFailPercent < 0 || play.MaxFailPercent > 100 {
		return fmt.Errorf("play %q 的失败阈值参数非法", play.Name)
	}
	return nil
}

func validateTask(task Task, vars Vars) error {
	hasModule := task.Module != ""
	hasBlock := len(task.Block) > 0
	hasInclude := task.Include != ""
	modes := 0
	for _, present := range []bool{hasModule, hasBlock, hasInclude} {
		if present {
			modes++
		}
	}
	if modes != 1 {
		return fmt.Errorf("task 必须且只能包含 module、include 或 block 之一")
	}
	if task.When != "" {
		if _, err := EvalWhen(task.When, map[string]any{}); err != nil {
			return err
		}
	}
	if task.Module != "" {
		module, ok := Lookup(task.Module)
		if !ok {
			return fmt.Errorf("未知模块 %q（可用: %s）", task.Module, strings.Join(ModuleNames(), ", "))
		}
		if len(task.Loop) > 0 {
			for _, item := range task.Loop {
				itemVars := cloneVars(vars)
				itemVars["item"] = item
				itemRendered, err := renderArgs(task.Args, itemVars)
				if err != nil {
					return err
				}
				if _, err := module.DecodeArgs(itemRendered); err != nil {
					return fmt.Errorf("loop item %q: %w", item, err)
				}
			}
		} else {
			rendered, err := renderArgs(task.Args, vars)
			if err != nil {
				return err
			}
			if _, err := module.DecodeArgs(rendered); err != nil {
				return err
			}
		}
	}
	for _, child := range append(append(task.Block, task.Rescue...), task.Always...) {
		if err := validateTask(child, vars); err != nil {
			return err
		}
	}
	return nil
}

func resolveVars(play Play, catalog *Catalog, overrides Overrides) (Vars, error) {
	fileVars := Vars{}
	for _, file := range catalog.Files {
		if file.Path == play.Source {
			fileVars = file.Vars
		}
	}
	vars := MergeVars(fileVars)
	for _, path := range play.VarsFiles {
		resolved := resolveRelative(play.BaseDir, path)
		loaded, err := loadVarsFile(resolved)
		if err != nil {
			return nil, err
		}
		vars = MergeVars(vars, loaded)
	}
	vars = MergeVars(vars, play.Vars)
	vars = MergeVars(vars, overrides.ExtraVars)
	return vars, nil
}

func loadVarsFile(path string) (Vars, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 vars 文件 %s 失败: %w", path, err)
	}
	var vars Vars
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&vars); err != nil {
		return nil, fmt.Errorf("解析 vars 文件 %s 失败: %w", path, err)
	}
	return vars, nil
}

// expandIncludes splices include tasks recursively. Each resulting task keeps
// the BaseDir of the file that defined it.
func expandIncludes(tasks []Task, baseDir string, stack []string, depth int) ([]Task, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("include 嵌套超过 %d 层", maxIncludeDepth)
	}
	var out []Task
	for _, task := range tasks {
		if task.BaseDir == "" {
			task.BaseDir = baseDir
		}
		if task.Include != "" {
			path := resolveRelative(task.BaseDir, task.Include)
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			for _, seen := range stack {
				if seen == absolute {
					return nil, fmt.Errorf("include 循环: %s", strings.Join(append(stack, absolute), " -> "))
				}
			}
			expanded, err := loadTaskFragment(absolute)
			if err != nil {
				return nil, err
			}
			nested, err := expandIncludes(expanded, filepath.Dir(absolute), append(stack, absolute), depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
			continue
		}
		for index := range task.Block {
			if task.Block[index].BaseDir == "" {
				task.Block[index].BaseDir = task.BaseDir
			}
		}
		for index := range task.Rescue {
			if task.Rescue[index].BaseDir == "" {
				task.Rescue[index].BaseDir = task.BaseDir
			}
		}
		for index := range task.Always {
			if task.Always[index].BaseDir == "" {
				task.Always[index].BaseDir = task.BaseDir
			}
		}
		out = append(out, task)
	}
	return out, nil
}

func loadTaskFragment(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 include 片段 %s 失败: %w", path, err)
	}
	var wrapped struct {
		Tasks []Task `yaml:"tasks"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&wrapped); err == nil && len(wrapped.Tasks) > 0 {
		return wrapped.Tasks, nil
	}
	var tasks []Task
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&tasks); err != nil {
		return nil, fmt.Errorf("解析 include 片段 %s 失败（支持 tasks: 列表或裸任务列表）: %w", path, err)
	}
	return tasks, nil
}

func resolveRelative(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

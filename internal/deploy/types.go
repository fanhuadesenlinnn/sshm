package deploy

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

const Version = 3

// Vars is a flat variable map used for playbook interpolation.
type Vars map[string]any

// File is the version-3 deploy document: global vars plus one or more plays.
type File struct {
	Version int    `yaml:"version" json:"version"`
	Vars    Vars   `yaml:"vars,omitempty" json:"vars,omitempty"`
	Plays   []Play `yaml:"plays" json:"plays"`

	Path    string `yaml:"-" json:"-"`
	BaseDir string `yaml:"-" json:"-"`
}

// Play declares a named workflow over a target set.
type Play struct {
	Name           string          `yaml:"name" json:"name"`
	Description    string          `yaml:"description,omitempty" json:"description,omitempty"`
	Hosts          TargetSelector  `yaml:"hosts" json:"hosts"`
	Strategy       string          `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	Serial         int             `yaml:"serial,omitempty" json:"serial,omitempty"`
	Parallel       int             `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Timeout        config.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	ConnectTimeout config.Duration `yaml:"connect_timeout,omitempty" json:"connect_timeout,omitempty"`
	FailFast       bool            `yaml:"fail_fast,omitempty" json:"fail_fast,omitempty"`
	MaxFail        int             `yaml:"max_fail,omitempty" json:"max_fail,omitempty"`
	MaxFailPercent int             `yaml:"max_fail_percent,omitempty" json:"max_fail_percent,omitempty"`
	GatherFacts    bool            `yaml:"gather_facts,omitempty" json:"gather_facts,omitempty"`
	Vars           Vars            `yaml:"vars,omitempty" json:"vars,omitempty"`
	VarsFiles      []string        `yaml:"vars_files,omitempty" json:"vars_files,omitempty"`
	Tasks          []Task          `yaml:"tasks" json:"tasks"`

	Source  string `yaml:"-" json:"source"`
	BaseDir string `yaml:"-" json:"-"`
}

// StrategyLinear waits for every host to finish a task before starting the next.
const StrategyLinear = "linear"

// StrategyFree runs every task on a host before moving to the next host.
const StrategyFree = "free"

// Task is a unit of execution. Exactly one of Module/Include/Block is set.
type Task struct {
	Name         string            `yaml:"name" json:"name"`
	Include      string            `yaml:"include,omitempty" json:"include,omitempty"`
	When         string            `yaml:"when,omitempty" json:"when,omitempty"`
	Register     string            `yaml:"register,omitempty" json:"register,omitempty"`
	Become       bool              `yaml:"become,omitempty" json:"become,omitempty"`
	BecomeUser   string            `yaml:"become_user,omitempty" json:"become_user,omitempty"`
	IgnoreErrors bool              `yaml:"ignore_errors,omitempty" json:"ignore_errors,omitempty"`
	Env          map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	FailedWhen   *Condition        `yaml:"failed_when,omitempty" json:"failed_when,omitempty"`
	ChangedWhen  *Condition        `yaml:"changed_when,omitempty" json:"changed_when,omitempty"`
	CheckSafe    bool              `yaml:"check_safe,omitempty" json:"check_safe,omitempty"`
	RunOnce      bool              `yaml:"run_once,omitempty" json:"run_once,omitempty"`
	Loop         []string          `yaml:"loop,omitempty" json:"loop,omitempty"`
	Confirm      string            `yaml:"confirm,omitempty" json:"confirm,omitempty"`

	Module string     `yaml:"-" json:"module,omitempty"`
	Args   *yaml.Node `yaml:"-" json:"-"`
	Block  []Task     `yaml:"block,omitempty" json:"block,omitempty"`
	Rescue []Task     `yaml:"rescue,omitempty" json:"rescue,omitempty"`
	Always []Task     `yaml:"always,omitempty" json:"always,omitempty"`

	BaseDir string `yaml:"-" json:"-"`
	Source  string `yaml:"-" json:"-"`
	// ProjectRoot is the directory of the top-level playbook. Included task
	// files may change BaseDir, but local reads must remain below this root.
	ProjectRoot string `yaml:"-" json:"-"`
}

// UnmarshalYAML accepts arbitrary module names as mapping keys while decoding
// the known task fields strictly. Unknown keys are treated as module names.
func (t *Task) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("task 必须是映射")
	}
	raw := map[string]yaml.Node{}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if _, ok := raw["become_password"]; ok {
		return fmt.Errorf("deploy 文件禁止保存 become_password；请使用 SSHMD_BECOME_PASSWORD 或主配置 vault")
	}
	decodeString := func(key string, target *string) error {
		value, ok := raw[key]
		if !ok {
			return nil
		}
		return value.Decode(target)
	}
	decodeBool := func(key string, target *bool) error {
		value, ok := raw[key]
		if !ok {
			return nil
		}
		return value.Decode(target)
	}
	if err := decodeString("name", &t.Name); err != nil {
		return fieldError("name", err)
	}
	if err := decodeString("include", &t.Include); err != nil {
		return fieldError("include", err)
	}
	if err := decodeString("when", &t.When); err != nil {
		return fieldError("when", err)
	}
	if err := decodeString("register", &t.Register); err != nil {
		return fieldError("register", err)
	}
	if err := decodeString("confirm", &t.Confirm); err != nil {
		return fieldError("confirm", err)
	}
	if err := decodeString("become_user", &t.BecomeUser); err != nil {
		return fieldError("become_user", err)
	}
	if err := decodeBool("become", &t.Become); err != nil {
		return fieldError("become", err)
	}
	if err := decodeBool("ignore_errors", &t.IgnoreErrors); err != nil {
		return fieldError("ignore_errors", err)
	}
	if err := decodeBool("check_safe", &t.CheckSafe); err != nil {
		return fieldError("check_safe", err)
	}
	if err := decodeBool("run_once", &t.RunOnce); err != nil {
		return fieldError("run_once", err)
	}
	if value, ok := raw["loop"]; ok {
		if err := value.Decode(&t.Loop); err != nil {
			return fieldError("loop", err)
		}
	}
	if value, ok := raw["env"]; ok {
		if err := value.Decode(&t.Env); err != nil {
			return fieldError("env", err)
		}
	}
	for _, key := range []string{"failed_when", "changed_when"} {
		if value, ok := raw[key]; ok {
			condition := &Condition{}
			if err := value.Decode(condition); err != nil {
				return fieldError(key, err)
			}
			if key == "failed_when" {
				t.FailedWhen = condition
			} else {
				t.ChangedWhen = condition
			}
		}
	}
	for _, key := range []string{"block", "rescue", "always"} {
		if value, ok := raw[key]; ok {
			var tasks []Task
			if err := value.Decode(&tasks); err != nil {
				return fieldError(key, err)
			}
			switch key {
			case "block":
				t.Block = tasks
			case "rescue":
				t.Rescue = tasks
			case "always":
				t.Always = tasks
			}
		}
	}
	if _, ok := raw["notify"]; ok {
		return fmt.Errorf("v3 已移除 notify/handlers；请使用 register + when 表达条件执行")
	}
	for key, value := range raw {
		switch key {
		case "name", "include", "when", "register", "confirm", "become_user", "become_password", "become",
			"ignore_errors", "check_safe", "run_once", "loop", "env",
			"failed_when", "changed_when", "block", "rescue", "always", "notify":
			continue
		}
		if t.Module != "" {
			return fmt.Errorf("task %q 只能包含一个模块，出现 %q 和 %q", t.Name, t.Module, key)
		}
		if value.Kind != yaml.MappingNode {
			return fmt.Errorf("模块 %q 的参数必须是映射", key)
		}
		t.Module = key
		copy := value
		t.Args = &copy
	}
	return nil
}

func fieldError(field string, err error) error {
	return fmt.Errorf("字段 %s: %w", field, err)
}

// DisplayName returns the task name or a module-based fallback.
func (t Task) DisplayName(index int) string {
	if t.Name != "" {
		return t.Name
	}
	if t.Module != "" {
		return t.Module + "-" + itoa(index+1)
	}
	if len(t.Block) > 0 {
		return "block-" + itoa(index+1)
	}
	return "task-" + itoa(index+1)
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}

// Catalog is the merged set of version-3 playbooks.
type Catalog struct {
	Sources []string
	Files   []*File
	Plays   []Play
	ByName  map[string]Play
}

func (c *Catalog) PlayNames() []string {
	names := make([]string, 0, len(c.Plays))
	for _, play := range c.Plays {
		names = append(names, play.Name)
	}
	sort.Strings(names)
	return names
}

func (p Play) StrategyOrDefault() string {
	if p.Strategy == "" {
		return StrategyLinear
	}
	return strings.ToLower(p.Strategy)
}

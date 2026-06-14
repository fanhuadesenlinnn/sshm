package deploy

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"gopkg.in/yaml.v3"
)

const Version = 1

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return err
	}
	switch typed := value.(type) {
	case string:
		parsed, err := time.ParseDuration(typed)
		if err != nil {
			return fmt.Errorf("无效时长 %q: %w", typed, err)
		}
		d.Duration = parsed
		return nil
	default:
		return fmt.Errorf("时长必须使用 30s、5m 等格式")
	}
}

func (d Duration) MarshalYAML() (any, error) {
	if d.Duration == 0 {
		return "", nil
	}
	return d.String(), nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(d.String())), nil
}

type File struct {
	Version     int               `yaml:"version"`
	Name        string            `yaml:"name,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Vars        map[string]string `yaml:"vars,omitempty"`
	Defaults    Defaults          `yaml:"defaults,omitempty"`
	Profiles    []Profile         `yaml:"profiles"`

	Path    string `yaml:"-" json:"-"`
	BaseDir string `yaml:"-" json:"-"`
}

type Defaults struct {
	Strategy Strategy `yaml:"strategy,omitempty"`
}

type Profile struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	Targets     TargetSelector `yaml:"targets,omitempty"`
	Strategy    Strategy       `yaml:"strategy,omitempty"`
	Steps       []Step         `yaml:"steps"`

	Source  string            `yaml:"-" json:"source"`
	BaseDir string            `yaml:"-" json:"-"`
	Vars    map[string]string `yaml:"-" json:"-"`
}

type TargetSelector struct {
	Hosts []string `yaml:"hosts,omitempty" json:"hosts,omitempty"`
	Tags  []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	All   bool     `yaml:"all,omitempty" json:"all,omitempty"`
}

func (s TargetSelector) Empty() bool {
	return !s.All && len(s.Hosts) == 0 && len(s.Tags) == 0
}

func (s TargetSelector) String() string {
	if s.All {
		return "all"
	}
	value := ""
	if len(s.Hosts) > 0 {
		value = "host:" + join(s.Hosts)
	}
	if len(s.Tags) > 0 {
		if value != "" {
			value += " + "
		}
		value += "tag:" + join(s.Tags)
	}
	return value
}

type Strategy struct {
	Mode           string   `yaml:"mode,omitempty" json:"mode"`
	MaxParallel    int      `yaml:"max_parallel,omitempty" json:"max_parallel"`
	ConnectTimeout Duration `yaml:"connect_timeout,omitempty" json:"connect_timeout"`
	StepTimeout    Duration `yaml:"step_timeout,omitempty" json:"step_timeout"`
	RunTimeout     Duration `yaml:"run_timeout,omitempty" json:"run_timeout"`
	RetryCount     int      `yaml:"retry_count,omitempty" json:"retry_count"`
	RetryOnStage   []string `yaml:"retry_on_stage,omitempty" json:"retry_on_stage"`

	maxParallelSet bool
	retryCountSet  bool
}

func (s *Strategy) UnmarshalYAML(node *yaml.Node) error {
	allowed := map[string]bool{
		"mode": true, "max_parallel": true, "connect_timeout": true, "step_timeout": true,
		"run_timeout": true, "retry_count": true, "retry_on_stage": true,
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if key := node.Content[index].Value; !allowed[key] {
				return fmt.Errorf("未知 strategy 字段 %q", key)
			}
		}
	}
	type rawStrategy Strategy
	var raw rawStrategy
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*s = Strategy(raw)
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			switch node.Content[index].Value {
			case "max_parallel":
				s.maxParallelSet = true
			case "retry_count":
				s.retryCountSet = true
			}
		}
	}
	return nil
}

type Step struct {
	Name      string   `yaml:"name,omitempty" json:"name"`
	Type      string   `yaml:"type" json:"type"`
	Src       string   `yaml:"src,omitempty" json:"src,omitempty"`
	Dest      string   `yaml:"dest,omitempty" json:"dest,omitempty"`
	Method    string   `yaml:"method,omitempty" json:"method,omitempty"`
	Overwrite bool     `yaml:"overwrite,omitempty" json:"overwrite,omitempty"`
	Command   string   `yaml:"command,omitempty" json:"command,omitempty"`
	Timeout   Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

func (s Step) DisplayName(index int) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Type + "-" + strconv.Itoa(index+1)
}

type Catalog struct {
	Sources  []string
	Profiles []Profile
	ByName   map[string]Profile
}

type Plan struct {
	Profile     string         `json:"profile"`
	Description string         `json:"description,omitempty"`
	Config      string         `json:"config"`
	Sources     []string       `json:"sources"`
	Targets     TargetSelector `json:"selector"`
	Hosts       []config.Host  `json:"-"`
	Strategy    Strategy       `json:"strategy"`
	Steps       []Step         `json:"steps"`
}

type PlanJSON struct {
	Profile     string         `json:"profile"`
	Description string         `json:"description,omitempty"`
	Config      string         `json:"config"`
	Sources     []string       `json:"sources"`
	Selector    TargetSelector `json:"selector"`
	Targets     []PlanHost     `json:"targets"`
	Strategy    Strategy       `json:"strategy"`
	Steps       []Step         `json:"steps"`
}

type PlanHost struct {
	Alias   string `json:"alias"`
	Address string `json:"address"`
}

type StepResult struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	OK          bool                   `json:"ok"`
	Stage       operation.FailureStage `json:"stage,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Output      string                 `json:"output,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Destination string                 `json:"destination,omitempty"`
	Attempts    int                    `json:"attempts"`
	DurationMS  int64                  `json:"duration_ms"`
}

type HostResult struct {
	HostAlias    string                 `json:"host"`
	HostAddress  string                 `json:"address"`
	OK           bool                   `json:"ok"`
	FailedStep   string                 `json:"failed_step,omitempty"`
	Stage        operation.FailureStage `json:"stage,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	Suggestion   string                 `json:"suggestion,omitempty"`
	RetryCommand string                 `json:"retry_command,omitempty"`
	StartedAt    time.Time              `json:"started_at"`
	EndedAt      time.Time              `json:"ended_at"`
	Steps        []StepResult           `json:"steps"`
}

type RunResult struct {
	Profile   string       `json:"profile"`
	Config    string       `json:"config"`
	Mode      string       `json:"mode"`
	Targets   int          `json:"targets"`
	OK        int          `json:"ok"`
	Failed    int          `json:"failed"`
	Cancelled bool         `json:"cancelled"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   time.Time    `json:"ended_at"`
	Results   []HostResult `json:"results"`
	LogPath   string       `json:"log_path,omitempty"`
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}

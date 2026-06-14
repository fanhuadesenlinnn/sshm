package deploy

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

const Version = 2

type Duration = config.Duration

type File struct {
	Version  int       `yaml:"version" json:"version"`
	Profiles []Profile `yaml:"profiles" json:"profiles"`
	Handlers []Step    `yaml:"handlers,omitempty" json:"handlers,omitempty"`

	Path    string `yaml:"-" json:"-"`
	BaseDir string `yaml:"-" json:"-"`
}

type Profile struct {
	Name           string         `yaml:"name" json:"name"`
	Description    string         `yaml:"description,omitempty" json:"description,omitempty"`
	Targets        TargetSelector `yaml:"targets" json:"targets"`
	Serial         int            `yaml:"serial,omitempty" json:"serial,omitempty"`
	Parallel       int            `yaml:"parallel,omitempty" json:"parallel,omitempty"`
	Timeout        Duration       `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	ConnectTimeout Duration       `yaml:"connect_timeout,omitempty" json:"connect_timeout,omitempty"`
	FailFast       bool           `yaml:"fail_fast,omitempty" json:"fail_fast,omitempty"`
	MaxFail        int            `yaml:"max_fail,omitempty" json:"max_fail,omitempty"`
	MaxFailPercent int            `yaml:"max_fail_percent,omitempty" json:"max_fail_percent,omitempty"`
	Steps          []Step         `yaml:"steps" json:"steps"`

	Source  string `yaml:"-" json:"source"`
	BaseDir string `yaml:"-" json:"-"`
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

type Step struct {
	Name        string       `yaml:"name" json:"name"`
	Timeout     Duration     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Notify      []string     `yaml:"notify,omitempty" json:"notify,omitempty"`
	IgnoreError bool         `yaml:"ignore_error,omitempty" json:"ignore_error,omitempty"`
	Exec        string       `yaml:"exec,omitempty" json:"exec,omitempty"`
	Push        *PushAction  `yaml:"push,omitempty" json:"push,omitempty"`
	Pull        *PullAction  `yaml:"pull,omitempty" json:"pull,omitempty"`
	Mkdir       *MkdirAction `yaml:"mkdir,omitempty" json:"mkdir,omitempty"`
	Wait        *Duration    `yaml:"wait,omitempty" json:"wait,omitempty"`
	Confirm     string       `yaml:"confirm,omitempty" json:"confirm,omitempty"`
	Become      bool         `yaml:"become,omitempty" json:"become,omitempty"`
	BecomeUser  string       `yaml:"become_user,omitempty" json:"become_user,omitempty"`
	CheckSafe   bool         `yaml:"check_safe,omitempty" json:"check_safe,omitempty"`
	FailedWhen  *Condition   `yaml:"failed_when,omitempty" json:"failed_when,omitempty"`
	ChangedWhen *Condition   `yaml:"changed_when,omitempty" json:"changed_when,omitempty"`

	BaseDir string `yaml:"-" json:"-"`
}

type PushAction struct {
	Src       string `yaml:"src" json:"src"`
	Dest      string `yaml:"dest" json:"dest"`
	Checksum  *bool  `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	Overwrite bool   `yaml:"overwrite,omitempty" json:"overwrite,omitempty"`
	Backup    bool   `yaml:"backup,omitempty" json:"backup,omitempty"`
}

type PullAction struct {
	Src       string `yaml:"src" json:"src"`
	Dest      string `yaml:"dest" json:"dest"`
	Flat      bool   `yaml:"flat,omitempty" json:"flat,omitempty"`
	Checksum  *bool  `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	Overwrite bool   `yaml:"overwrite,omitempty" json:"overwrite,omitempty"`
	Backup    bool   `yaml:"backup,omitempty" json:"backup,omitempty"`
}

type MkdirAction struct {
	Path string `yaml:"path" json:"path"`
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
}

type Condition struct {
	RCIn    []int `yaml:"rc_in,omitempty" json:"rc_in,omitempty"`
	RCNotIn []int `yaml:"rc_not_in,omitempty" json:"rc_not_in,omitempty"`
}

func (c Condition) Matches(rc int) bool {
	for _, value := range c.RCIn {
		if rc == value {
			return true
		}
	}
	if len(c.RCNotIn) > 0 {
		for _, value := range c.RCNotIn {
			if rc == value {
				return false
			}
		}
		return true
	}
	return false
}

func (s Step) ActionType() string {
	switch {
	case s.Exec != "":
		return "exec"
	case s.Push != nil:
		return "push"
	case s.Pull != nil:
		return "pull"
	case s.Mkdir != nil:
		return "mkdir"
	case s.Wait != nil:
		return "wait"
	case s.Confirm != "":
		return "confirm"
	default:
		return ""
	}
}

func (s Step) DisplayName(index int) string {
	if s.Name != "" {
		return s.Name
	}
	action := s.ActionType()
	if action == "" {
		action = "step"
	}
	return action + "-" + strconv.Itoa(index+1)
}

func (a PushAction) ValidateChecksum() bool {
	return a.Checksum == nil || *a.Checksum
}

func (a PullAction) ValidateChecksum() bool {
	return a.Checksum == nil || *a.Checksum
}

type Catalog struct {
	Sources       []string
	Profiles      []Profile
	Handlers      []Step
	ByName        map[string]Profile
	HandlerByName map[string]Step
}

type Overrides struct {
	Targets               *TargetSelector
	Parallel              int
	Serial                int
	FailFast              bool
	MaxFail               int
	MaxFailPercent        int
	Check                 bool
	Diff                  bool
	DefaultParallel       int
	DefaultTimeout        Duration
	DefaultConnectTimeout Duration
}

type Plan struct {
	Profile        string         `json:"profile"`
	Description    string         `json:"description,omitempty"`
	Config         string         `json:"config"`
	Sources        []string       `json:"sources"`
	Targets        TargetSelector `json:"selector"`
	Hosts          []config.Host  `json:"-"`
	Batch          batch.Options  `json:"batch"`
	Timeout        Duration       `json:"timeout"`
	ConnectTimeout Duration       `json:"connect_timeout"`
	Check          bool           `json:"check"`
	Diff           bool           `json:"diff"`
	Steps          []Step         `json:"steps"`
	Handlers       []Step         `json:"handlers,omitempty"`
}

type PlanJSON struct {
	Profile        string         `json:"profile"`
	Description    string         `json:"description,omitempty"`
	Config         string         `json:"config"`
	Sources        []string       `json:"sources"`
	Selector       TargetSelector `json:"selector"`
	Targets        []PlanHost     `json:"targets"`
	Batch          batch.Options  `json:"batch"`
	Timeout        Duration       `json:"timeout"`
	ConnectTimeout Duration       `json:"connect_timeout"`
	Check          bool           `json:"check"`
	Diff           bool           `json:"diff"`
	Steps          []Step         `json:"steps"`
	Handlers       []Step         `json:"handlers,omitempty"`
}

type PlanHost struct {
	Alias   string `json:"alias"`
	Address string `json:"address"`
}

type StepResult struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Status      batch.Status           `json:"status"`
	Ignored     bool                   `json:"ignored,omitempty"`
	Stage       operation.FailureStage `json:"stage,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Output      string                 `json:"output,omitempty"`
	Destination string                 `json:"destination,omitempty"`
	RC          int                    `json:"rc"`
	DurationMS  int64                  `json:"duration_ms"`
}

type HostResult struct {
	HostAlias    string                 `json:"host"`
	HostAddress  string                 `json:"address"`
	Status       batch.Status           `json:"status"`
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
	Profile    string        `json:"profile"`
	Config     string        `json:"config"`
	Targets    int           `json:"targets"`
	Summary    batch.Summary `json:"summary"`
	Cancelled  bool          `json:"cancelled"`
	StopReason string        `json:"stop_reason,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	EndedAt    time.Time     `json:"ended_at"`
	Results    []HostResult  `json:"results"`
	LogPath    string        `json:"log_path,omitempty"`
}

func (r RunResult) ExitCode() int {
	switch {
	case r.Cancelled:
		return 130
	case r.Summary.Failed > 0 || r.Summary.Skipped > 0:
		return 1
	case r.Summary.Unreachable > 0:
		return 2
	default:
		return 0
	}
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

func (p Plan) Validate() error {
	if err := p.Batch.Validate(); err != nil {
		return err
	}
	if p.Timeout.Duration <= 0 || p.ConnectTimeout.Duration <= 0 {
		return fmt.Errorf("deploy timeout 和 connect_timeout 必须大于 0")
	}
	return nil
}

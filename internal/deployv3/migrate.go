package deployv3

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
)

// Migration is the result of converting v2 profiles to v3 plays.
type Migration struct {
	YAML      []byte
	Warnings  []string
	PlayCount int
}

// MigrateFromV2 converts every v2 profile in the given files to v3 plays.
func MigrateFromV2(paths []string) (*Migration, error) {
	catalog, err := deploy.Load(paths)
	if err != nil {
		return nil, err
	}
	file := File{Version: Version}
	var warnings []string
	for _, profile := range catalog.Profiles {
		play := Play{
			Name: profile.Name, Description: profile.Description,
			Hosts: profile.Targets, Serial: profile.Serial, Parallel: profile.Parallel,
			Timeout: profile.Timeout, ConnectTimeout: profile.ConnectTimeout,
			FailFast: profile.FailFast, MaxFail: profile.MaxFail, MaxFailPercent: profile.MaxFailPercent,
			Source: profile.Source, BaseDir: profile.BaseDir,
		}
		for index, step := range profile.Steps {
			task, warning, err := migrateStep(step)
			if err != nil {
				return nil, fmt.Errorf("profile %q step %d: %w", profile.Name, index+1, err)
			}
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("profile %q: %s", profile.Name, warning))
			}
			play.Tasks = append(play.Tasks, task)
		}
		for _, handler := range catalog.Handlers {
			warnings = append(warnings, fmt.Sprintf("profile %q: handler %q 已移除，请用 register + when 重写", profile.Name, handler.Name))
		}
		file.Plays = append(file.Plays, play)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return nil, err
	}
	return &Migration{YAML: data, Warnings: warnings, PlayCount: len(file.Plays)}, nil
}

func migrateStep(step deploy.Step) (Task, string, error) {
	task := Task{
		Name: step.Name, When: "", Become: step.Become, BecomeUser: step.BecomeUser,
		IgnoreErrors: step.IgnoreError, CheckSafe: step.CheckSafe,
		FailedWhen: step.FailedWhen, ChangedWhen: step.ChangedWhen,
		BaseDir: step.BaseDir,
	}
	warning := ""
	switch step.ActionType() {
	case "exec":
		task.Module = "command"
		task.Args = argsNode(map[string]any{"cmd": step.Exec})
	case "push":
		task.Module = "copy"
		task.Args = argsNode(map[string]any{
			"src": step.Push.Src, "dest": step.Push.Dest,
			"backup": step.Push.Backup, "checksum": step.Push.Checksum,
		})
	case "pull":
		task.Module = "fetch"
		task.Args = argsNode(map[string]any{
			"src": step.Pull.Src, "dest": step.Pull.Dest,
			"flat": step.Pull.Flat, "backup": step.Pull.Backup,
		})
	case "mkdir":
		task.Module = "file"
		task.Args = argsNode(map[string]any{"path": step.Mkdir.Path, "state": "directory", "mode": step.Mkdir.Mode})
	case "wait":
		task.Module = "wait_for"
		task.Args = argsNode(map[string]any{"timeout": step.Wait.String()})
	case "confirm":
		task.Module = "pause"
		task.Args = argsNode(map[string]any{"message": step.Confirm})
	default:
		return Task{}, "", fmt.Errorf("无法转换 action %q", step.ActionType())
	}
	if len(step.Notify) > 0 {
		warning = fmt.Sprintf("notify %s 已移除，请用 register + when 重写", strings.Join(step.Notify, ","))
	}
	return task, warning, nil
}

func argsNode(values map[string]any) *yaml.Node {
	node := &yaml.Node{}
	data, err := yaml.Marshal(values)
	if err != nil {
		return nil
	}
	if err := yaml.Unmarshal(data, node); err != nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	return node
}

// MarshalYAML emits a task with its module name as the mapping key.
func (t Task) MarshalYAML() (any, error) {
	out := map[string]any{}
	if t.Name != "" {
		out["name"] = t.Name
	}
	if t.When != "" {
		out["when"] = t.When
	}
	if t.Register != "" {
		out["register"] = t.Register
	}
	if t.Become {
		out["become"] = true
	}
	if t.BecomeUser != "" {
		out["become_user"] = t.BecomeUser
	}
	if t.IgnoreErrors {
		out["ignore_errors"] = true
	}
	if t.CheckSafe {
		out["check_safe"] = true
	}
	if t.RunOnce {
		out["run_once"] = true
	}
	if len(t.Loop) > 0 {
		out["loop"] = t.Loop
	}
	if len(t.Env) > 0 {
		out["env"] = t.Env
	}
	if t.FailedWhen != nil {
		out["failed_when"] = t.FailedWhen
	}
	if t.ChangedWhen != nil {
		out["changed_when"] = t.ChangedWhen
	}
	if len(t.Block) > 0 {
		out["block"] = t.Block
	}
	if len(t.Rescue) > 0 {
		out["rescue"] = t.Rescue
	}
	if len(t.Always) > 0 {
		out["always"] = t.Always
	}
	if t.Module != "" && t.Args != nil {
		var args any
		if err := t.Args.Decode(&args); err != nil {
			return nil, err
		}
		out[t.Module] = args
	}
	return out, nil
}

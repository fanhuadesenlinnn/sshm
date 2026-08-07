package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
	"gopkg.in/yaml.v3"
)

// PlanJSON is the machine-readable projection of a plan.
type PlanJSON struct {
	Name           string        `json:"name"`
	Config         string        `json:"config"`
	Strategy       string        `json:"strategy"`
	Targets        []string      `json:"targets"`
	Batch          batch.Options `json:"batch"`
	Timeout        string        `json:"timeout"`
	ConnectTimeout string        `json:"connect_timeout"`
	Check          bool          `json:"check"`
	Diff           bool          `json:"diff"`
	Tasks          []TaskJSON    `json:"tasks"`
}

// WriteJSON writes an indented JSON value.
func WriteJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

type TaskJSON struct {
	Name     string `json:"name,omitempty"`
	Module   string `json:"module,omitempty"`
	When     string `json:"when,omitempty"`
	Register string `json:"register,omitempty"`
	RunOnce  bool   `json:"run_once,omitempty"`
	Loop     int    `json:"loop,omitempty"`
	Block    int    `json:"block,omitempty"`
	Args     any    `json:"args,omitempty"`
}

// WritePlanJSON writes the plan as indented JSON.
func WritePlanJSON(writer io.Writer, plan *Plan) error {
	out := PlanJSON{
		Name: plan.Name, Config: plan.Config, Strategy: plan.Strategy,
		Batch: plan.Batch, Timeout: plan.Timeout.String(),
		ConnectTimeout: plan.ConnectTimeout.String(),
		Check:          plan.Check, Diff: plan.Diff,
	}
	for _, host := range plan.Hosts {
		out.Targets = append(out.Targets, fmt.Sprintf("%s %s@%s:%d", host.Alias, host.User, host.Host, host.Port))
	}
	for _, task := range plan.Tasks {
		out.Tasks = append(out.Tasks, TaskJSON{
			Name: task.DisplayName(0), Module: task.Module, When: task.When,
			Register: task.Register, RunOnce: task.RunOnce,
			Loop: len(task.Loop), Block: len(task.Block), Args: taskArgsJSON(task.Args),
		})
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}

// taskArgsJSON decodes the raw module argument node for machine-readable
// plan output.
func taskArgsJSON(node *yaml.Node) any {
	if node == nil {
		return nil
	}
	var out any
	if err := node.Decode(&out); err != nil {
		return nil
	}
	return out
}

// WritePlanText prints a human-readable execution plan.
func WritePlanText(writer io.Writer, plan *Plan) {
	fmt.Fprintf(writer, "Play: %s\nConfig: %s\n", plan.Name, plan.Config)
	if plan.Description != "" {
		fmt.Fprintf(writer, "Description: %s\n", plan.Description)
	}
	if len(plan.Sources) > 1 {
		fmt.Fprintf(writer, "Sources: %s\n", strings.Join(plan.Sources, ", "))
	}
	fmt.Fprintf(writer, "Strategy: %s\n", plan.Strategy)
	fmt.Fprintf(writer, "Mode: check=%t diff=%t\n", plan.Check, plan.Diff)
	fmt.Fprintf(writer, "Batch: serial=%d parallel=%d fail_fast=%t max_fail=%d max_fail_percent=%d\n",
		plan.Batch.Serial, plan.Batch.Parallel, plan.Batch.FailFast, plan.Batch.MaxFail, plan.Batch.MaxFailPercent)
	fmt.Fprintf(writer, "Timeouts: connect=%s task=%s\n", plan.ConnectTimeout, plan.Timeout)
	fmt.Fprintln(writer, "\nTargets:")
	for _, host := range plan.Hosts {
		fmt.Fprintf(writer, "  - %s %s@%s:%d\n", host.Alias, host.User, host.Host, host.Port)
	}
	fmt.Fprintln(writer, "\nTasks:")
	for index, task := range plan.Tasks {
		writeTaskLine(writer, index, task, "")
	}
}

func writeTaskLine(writer io.Writer, index int, task Task, prefix string) {
	label := task.DisplayName(index)
	module := task.Module
	if module == "" {
		module = "block"
	}
	extra := ""
	if task.When != "" {
		extra += " when=" + task.When
	}
	if task.Register != "" {
		extra += " register=" + task.Register
	}
	if len(task.Loop) > 0 {
		extra += fmt.Sprintf(" loop=%d", len(task.Loop))
	}
	if task.RunOnce {
		extra += " run_once"
	}
	fmt.Fprintf(writer, "  %s%d. [%s] %s%s\n", prefix, index+1, module, label, extra)
	for childIndex, child := range task.Block {
		writeTaskLine(writer, childIndex, child, prefix+"  block: ")
	}
	for childIndex, child := range task.Rescue {
		writeTaskLine(writer, childIndex, child, prefix+"  rescue: ")
	}
	for childIndex, child := range task.Always {
		writeTaskLine(writer, childIndex, child, prefix+"  always: ")
	}
}

// WriteRunText prints the aggregated run result.
func WriteRunText(writer io.Writer, result RunResult) {
	fmt.Fprintf(writer, "\nPlay: %s\nTargets: %d\n\n", result.Play, result.Targets)
	for index, host := range result.Hosts {
		fmt.Fprintf(writer, "[%d/%d] %-20s %s", index+1, len(result.Hosts), host.HostAlias, host.Status)
		if host.Reason != "" {
			fmt.Fprintf(writer, " stage=%s reason=%s", host.Stage, host.Reason)
		}
		fmt.Fprintln(writer)
		for _, task := range host.Tasks {
			fmt.Fprintf(writer, "  - %-24s %-12s %s", task.Name, task.Status, time.Duration(task.DurationMS)*time.Millisecond)
			if task.Ignored {
				fmt.Fprint(writer, " ignored")
			}
			if task.Reason != "" {
				fmt.Fprintf(writer, " reason=%s", task.Reason)
			}
			fmt.Fprintln(writer)
			if task.Output != "" {
				fmt.Fprint(writer, indent(task.Output, "      "))
			}
		}
		if host.Status == batch.StatusFailed || host.Status == batch.StatusUnreachable {
			fmt.Fprintf(writer, "  Suggestion: %s\n", host.Suggestion)
		}
	}
	fmt.Fprintf(writer, "\nSummary: ok=%d changed=%d would-change=%d failed=%d unreachable=%d skipped=%d\n",
		result.Summary.OK, result.Summary.Changed, result.Summary.WouldChange,
		result.Summary.Failed, result.Summary.Unreachable, result.Summary.Skipped)
	if result.Cancelled {
		fmt.Fprintln(writer, "Cancelled: true")
	}
	if result.StopReason != "" {
		fmt.Fprintf(writer, "Stop reason: %s\n", result.StopReason)
	}
	if result.LogPath != "" {
		fmt.Fprintf(writer, "Log: %s\n", result.LogPath)
	}
}

func indent(value, prefix string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	return prefix + strings.Join(lines, "\n"+prefix) + "\n"
}

// WriteLog persists the plan and result as JSON under the sshm logs directory.
func WriteLog(plan *Plan, result *RunResult, retention time.Duration) (string, error) {
	if err := operation.CleanExpired(retention); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405.000000000") + "-deploy-" + sanitize(plan.Name)
	dir := filepath.Join(config.LogsDir(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	var planBuffer strings.Builder
	if err := WritePlanJSON(&planBuffer, plan); err != nil {
		return "", err
	}
	if err := safefile.Write(filepath.Join(dir, "plan.json"), []byte(planBuffer.String()), 0600); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	if err := safefile.Write(filepath.Join(dir, "run.json"), append(data, '\n'), 0600); err != nil {
		return "", err
	}
	result.LogPath = dir
	return dir, nil
}

func sanitize(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

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
)

func WriteJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func WritePlanText(writer io.Writer, plan Plan) {
	fmt.Fprintf(writer, "Profile: %s\nConfig: %s\n", plan.Profile, plan.Config)
	if len(plan.Sources) > 1 {
		fmt.Fprintf(writer, "Sources: %s\n", strings.Join(plan.Sources, ", "))
	}
	if plan.Description != "" {
		fmt.Fprintf(writer, "Description: %s\n", plan.Description)
	}
	fmt.Fprintf(writer, "Mode: check=%t diff=%t\n", plan.Check, plan.Diff)
	fmt.Fprintf(writer, "Batch: serial=%d parallel=%d fail_fast=%t max_fail=%d max_fail_percent=%d\n",
		plan.Batch.Serial, plan.Batch.Parallel, plan.Batch.FailFast, plan.Batch.MaxFail, plan.Batch.MaxFailPercent)
	fmt.Fprintf(writer, "Timeouts: connect=%s step=%s\n", plan.ConnectTimeout, plan.Timeout)
	fmt.Fprintln(writer, "\nTargets:")
	for _, host := range plan.Hosts {
		fmt.Fprintf(writer, "  - %s %s@%s:%d\n", host.Alias, host.User, host.Host, host.Port)
	}
	fmt.Fprintln(writer, "\nSteps:")
	writeSteps(writer, plan.Steps)
	if len(plan.Handlers) > 0 {
		fmt.Fprintln(writer, "\nHandlers:")
		writeSteps(writer, plan.Handlers)
	}
}

func writeSteps(writer io.Writer, steps []Step) {
	for index, step := range steps {
		detail := step.ActionType()
		switch step.ActionType() {
		case "exec":
			detail += " " + step.Exec
		case "push":
			detail += fmt.Sprintf(" %s -> %s", step.Push.Src, step.Push.Dest)
		case "pull":
			detail += fmt.Sprintf(" %s -> %s", step.Pull.Src, step.Pull.Dest)
		case "mkdir":
			detail += " " + step.Mkdir.Path
		case "wait":
			detail += " " + step.Wait.String()
		case "confirm":
			detail += "(batch gate) " + step.Confirm
		}
		if step.Timeout.Duration > 0 {
			detail += " timeout=" + step.Timeout.String()
		}
		if len(step.Notify) > 0 {
			detail += " notify=" + strings.Join(step.Notify, ",")
		}
		fmt.Fprintf(writer, "  %d. [%s] %s\n", index+1, step.DisplayName(index), detail)
	}
}

func WriteRunText(writer io.Writer, result RunResult) {
	fmt.Fprintf(writer, "\nDeploy: %s\nTargets: %d\n\n", result.Profile, result.Targets)
	for index, host := range result.Results {
		fmt.Fprintf(writer, "[%d/%d] %-20s %s", index+1, len(result.Results), host.HostAlias, host.Status)
		if host.Reason != "" {
			fmt.Fprintf(writer, " stage=%s reason=%s", host.Stage, host.Reason)
		}
		fmt.Fprintln(writer)
		for _, step := range host.Steps {
			fmt.Fprintf(writer, "  - %-24s %-12s %s", step.Name, step.Status, time.Duration(step.DurationMS)*time.Millisecond)
			if step.Ignored {
				fmt.Fprint(writer, " ignored")
			}
			if step.Reason != "" {
				fmt.Fprintf(writer, " reason=%s", step.Reason)
			}
			fmt.Fprintln(writer)
			if step.Output != "" {
				fmt.Fprint(writer, indent(step.Output, "      "))
			}
		}
		if host.Status == batch.StatusFailed || host.Status == batch.StatusUnreachable {
			fmt.Fprintf(writer, "  Suggestion: %s\n  Retry: %s\n", host.Suggestion, host.RetryCommand)
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

func WriteLog(plan Plan, result *RunResult, retention time.Duration) (string, error) {
	if err := operation.CleanExpired(retention); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405.000000000") + "-deploy-" + sanitize(plan.Profile)
	dir := filepath.Join(config.LogsDir(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	planData, err := json.MarshalIndent(plan.JSON(), "", "  ")
	if err != nil {
		return "", err
	}
	if err := safefile.Write(filepath.Join(dir, "plan.json"), append(planData, '\n'), 0600); err != nil {
		return "", err
	}
	logResult := loggableRunResult(plan, *result)
	for _, logHost := range logResult.Results {
		data, err := json.MarshalIndent(logHost, "", "  ")
		if err != nil {
			return "", err
		}
		if err := safefile.Write(filepath.Join(dir, sanitize(logHost.HostAlias)+".json"), append(data, '\n'), 0600); err != nil {
			return "", err
		}
	}
	result.LogPath = dir
	logResult.LogPath = dir
	data, err := json.MarshalIndent(logResult, "", "  ")
	if err != nil {
		return "", err
	}
	if err := safefile.Write(filepath.Join(dir, "summary.json"), append(data, '\n'), 0600); err != nil {
		return "", err
	}
	return dir, nil
}

func loggableRunResult(plan Plan, result RunResult) RunResult {
	result.Results = append([]HostResult(nil), result.Results...)
	for hostIndex := range result.Results {
		result.Results[hostIndex].Steps = append([]StepResult(nil), result.Results[hostIndex].Steps...)
		if !plan.Diff {
			continue
		}
		for stepIndex := range result.Results[hostIndex].Steps {
			step := &result.Results[hostIndex].Steps[stepIndex]
			if step.Type == "push" || step.Type == "pull" {
				step.Output = ""
			}
		}
	}
	return result
}

func indent(value, prefix string) string {
	lines := strings.SplitAfter(value, "\n")
	var out strings.Builder
	for _, line := range lines {
		if line != "" {
			out.WriteString(prefix)
			out.WriteString(line)
		}
	}
	if value != "" && !strings.HasSuffix(value, "\n") {
		out.WriteByte('\n')
	}
	return out.String()
}

func sanitize(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

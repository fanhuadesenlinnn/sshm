package deploy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/safefile"
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
	fmt.Fprintln(writer, "\nTargets:")
	for _, host := range plan.Hosts {
		fmt.Fprintf(writer, "  - %s %s@%s:%d\n", host.Alias, host.User, host.Host, host.Port)
	}
	fmt.Fprintf(writer, "\nStrategy:\n  mode: %s\n  max_parallel: %d\n  connect_timeout: %s\n  step_timeout: %s\n  run_timeout: %s\n  retry_count: %d\n  retry_on_stage: %s\n",
		plan.Strategy.Mode, plan.Strategy.MaxParallel, plan.Strategy.ConnectTimeout, plan.Strategy.StepTimeout,
		plan.Strategy.RunTimeout, plan.Strategy.RetryCount, strings.Join(plan.Strategy.RetryOnStage, ","))
	fmt.Fprintln(writer, "\nSteps:")
	for index, step := range plan.Steps {
		name := step.DisplayName(index)
		timeout := ""
		if step.Timeout.Duration > 0 {
			timeout = " timeout=" + step.Timeout.String()
		}
		if step.Type == "copy" {
			fmt.Fprintf(writer, "  %d. [%s] copy %s -> %s method=%s overwrite=%t%s\n", index+1, name, step.Src, step.Dest, step.Method, step.Overwrite, timeout)
		} else {
			fmt.Fprintf(writer, "  %d. [%s] exec %s%s\n", index+1, name, step.Command, timeout)
		}
	}
}

func WriteRunText(writer io.Writer, result RunResult) {
	fmt.Fprintf(writer, "\nDeploy: %s\nTargets: %d\nMode: %s\n\n", result.Profile, result.Targets, result.Mode)
	for index, host := range result.Results {
		status := "OK"
		if !host.OK {
			status = fmt.Sprintf("FAILED stage=%s reason=%s", host.Stage, host.Reason)
		}
		var steps []string
		for _, step := range host.Steps {
			steps = append(steps, fmt.Sprintf("%s=%s", step.Name, time.Duration(step.DurationMS)*time.Millisecond))
		}
		if len(steps) > 0 {
			status += " " + strings.Join(steps, " ")
		}
		fmt.Fprintf(writer, "[%d/%d] %s %s\n", index+1, len(result.Results), host.HostAlias, status)
		if !host.OK {
			fmt.Fprintf(writer, "  Suggestion: %s\n  Retry: %s\n", host.Suggestion, host.RetryCommand)
		}
	}
	fmt.Fprintf(writer, "\nSummary:\n  OK: %d\n  Failed: %d\n", result.OK, result.Failed)
	if result.Cancelled {
		fmt.Fprintln(writer, "  Cancelled: true")
	}
	if result.LogPath != "" {
		fmt.Fprintf(writer, "  Log: %s\n", result.LogPath)
	}
}

func WriteLog(plan Plan, result *RunResult) (string, error) {
	if err := operation.CleanExpired(30 * 24 * time.Hour); err != nil {
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
	for _, host := range result.Results {
		data, err := json.MarshalIndent(host, "", "  ")
		if err != nil {
			return "", err
		}
		if err := safefile.Write(filepath.Join(dir, sanitize(host.HostAlias)+".json"), append(data, '\n'), 0600); err != nil {
			return "", err
		}
	}
	result.LogPath = dir
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	if err := safefile.Write(filepath.Join(dir, "summary.json"), append(data, '\n'), 0600); err != nil {
		return "", err
	}
	return dir, nil
}

func sanitize(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

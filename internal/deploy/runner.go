package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

type Runner struct {
	Executor ops.Executor
	Visible  io.Writer
	Progress func(done, total int, result HostResult)
	Confirm  func(message string) error
}

func (r Runner) Run(ctx context.Context, plan Plan) RunResult {
	result := RunResult{
		Profile: plan.Profile, Config: plan.Config, Targets: len(plan.Hosts),
		StartedAt: time.Now(), Results: make([]HostResult, len(plan.Hosts)),
	}
	if err := preflightPullDestinations(plan); err != nil {
		return failedPlanResult(result, plan.Hosts, err)
	}
	batchResult, err := (batch.Runner{
		Options: plan.Batch,
		BeforeBatch: func(start, end, total int) error {
			return r.confirmBatch(plan, start, end, total)
		},
		Progress: func(done, total int, item batch.Result) {
			if r.Progress != nil {
				r.Progress(done, total, item.Value.(HostResult))
			}
		},
	}).Run(ctx, plan.Hosts, func(ctx context.Context, host config.Host) batch.Result {
		hostResult := r.runHost(ctx, host, plan)
		return batch.Result{
			Status: hostResult.Status, Err: hostResultError(hostResult), Value: hostResult,
		}
	})
	for index, item := range batchResult.Results {
		if item.Status == batch.StatusSkipped {
			now := time.Now()
			result.Results[index] = HostResult{
				HostAlias: item.Host.Alias, HostAddress: hostAddress(item.Host),
				Status: batch.StatusSkipped, Reason: item.SkippedReason, StartedAt: now, EndedAt: now,
			}
			continue
		}
		result.Results[index] = item.Value.(HostResult)
	}
	if err != nil {
		for index := range result.Results {
			if result.Results[index].Status != batch.StatusSkipped {
				continue
			}
			result.Results[index].Status = batch.StatusFailed
			result.Results[index].Stage = operation.StageConfig
			result.Results[index].Reason = err.Error()
			result.Results[index].Suggestion = operation.Suggestion(operation.StageConfig)
			break
		}
	}
	result.Summary = summarizeHostResults(result.Results)
	result.Cancelled = batchResult.Cancelled
	result.StopReason = batchResult.StopReason
	result.EndedAt = time.Now()
	return result
}

func failedPlanResult(result RunResult, hosts []config.Host, err error) RunResult {
	for index, host := range hosts {
		now := time.Now()
		result.Results[index] = HostResult{
			HostAlias: host.Alias, HostAddress: hostAddress(host), Status: batch.StatusFailed,
			Stage: operation.StageConfig, Reason: err.Error(), StartedAt: now, EndedAt: now,
		}
	}
	result.Summary.Failed = len(hosts)
	result.StopReason = err.Error()
	result.EndedAt = time.Now()
	return result
}

func preflightPullDestinations(plan Plan) error {
	steps := append(append([]Step(nil), plan.Steps...), plan.Handlers...)
	for index, step := range steps {
		if step.Pull == nil {
			continue
		}
		seen := map[string]string{}
		for _, host := range plan.Hosts {
			destination, err := deployPullDestination(step.Pull.Dest, host.Alias, step.Pull.Src, step.Pull.Flat)
			if err != nil {
				return fmt.Errorf("pull step %q: %w", step.DisplayName(index), err)
			}
			key := filepath.Clean(destination)
			if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
				key = strings.ToLower(key)
			}
			if previous, ok := seen[key]; ok {
				return fmt.Errorf("pull step %q 存在目标路径冲突: %s 与 %s", step.DisplayName(index), previous, destination)
			}
			seen[key] = destination
		}
	}
	return nil
}

func (r Runner) confirmBatch(plan Plan, start, end, total int) error {
	if plan.Check {
		return nil
	}
	seen := map[string]bool{}
	for _, step := range plan.Steps {
		if step.ActionType() != "confirm" || seen[step.Confirm] {
			continue
		}
		if r.Confirm == nil {
			return fmt.Errorf("deploy confirm step 需要交互确认: %s", step.Confirm)
		}
		message := fmt.Sprintf("%s（当前批次 %d-%d/%d）", step.Confirm, start+1, end, total)
		if err := r.Confirm(message); err != nil {
			return err
		}
		seen[step.Confirm] = true
	}
	return nil
}

func summarizeHostResults(results []HostResult) batch.Summary {
	var summary batch.Summary
	for _, result := range results {
		switch result.Status {
		case batch.StatusOK:
			summary.OK++
		case batch.StatusChanged:
			summary.Changed++
		case batch.StatusWouldChange:
			summary.WouldChange++
		case batch.StatusFailed:
			summary.Failed++
		case batch.StatusUnreachable:
			summary.Unreachable++
		case batch.StatusSkipped:
			summary.Skipped++
		}
	}
	return summary
}

func (r Runner) runHost(ctx context.Context, host config.Host, plan Plan) HostResult {
	hostResult := HostResult{
		HostAlias: host.Alias, HostAddress: hostAddress(host), Status: batch.StatusOK, StartedAt: time.Now(),
	}
	notified := map[string]bool{}
	for index, step := range plan.Steps {
		stepResult := r.runStep(ctx, host, plan, step, index)
		hostResult.Steps = append(hostResult.Steps, stepResult)
		if stepResult.Status == batch.StatusChanged {
			for _, name := range step.Notify {
				notified[name] = true
			}
		}
		if stepResult.Status == batch.StatusFailed || stepResult.Status == batch.StatusUnreachable {
			if step.IgnoreError {
				hostResult.Steps[len(hostResult.Steps)-1].Ignored = true
				continue
			}
			hostResult.Status = stepResult.Status
			hostResult.FailedStep = stepResult.Name
			hostResult.Stage = stepResult.Stage
			hostResult.Reason = stepResult.Reason
			hostResult.Suggestion = operation.Suggestion(stepResult.Stage)
			hostResult.RetryCommand = retryCommand(plan, host.Alias)
			hostResult.EndedAt = time.Now()
			return hostResult
		}
		hostResult.Status = aggregateStatus(hostResult.Status, stepResult.Status)
	}
	if !plan.Check {
		for index, handler := range plan.Handlers {
			if !notified[handler.Name] {
				continue
			}
			stepResult := r.runStep(ctx, host, plan, handler, index)
			stepResult.Name = "handler: " + handler.Name
			hostResult.Steps = append(hostResult.Steps, stepResult)
			if stepResult.Status == batch.StatusFailed || stepResult.Status == batch.StatusUnreachable {
				hostResult.Status = stepResult.Status
				hostResult.FailedStep = stepResult.Name
				hostResult.Stage = stepResult.Stage
				hostResult.Reason = stepResult.Reason
				hostResult.Suggestion = operation.Suggestion(stepResult.Stage)
				hostResult.RetryCommand = retryCommand(plan, host.Alias)
				hostResult.EndedAt = time.Now()
				return hostResult
			}
			hostResult.Status = aggregateStatus(hostResult.Status, stepResult.Status)
		}
	}
	hostResult.EndedAt = time.Now()
	return hostResult
}

func (r Runner) runStep(ctx context.Context, host config.Host, plan Plan, step Step, index int) StepResult {
	timeout := plan.Timeout.Duration
	if step.Timeout.Duration > 0 {
		timeout = step.Timeout.Duration
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	result := r.executeStep(stepCtx, host, plan, step)
	result.Name = step.DisplayName(index)
	result.Type = step.ActionType()
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func (r Runner) executeStep(ctx context.Context, host config.Host, plan Plan, step Step) StepResult {
	switch step.ActionType() {
	case "exec":
		return r.executeExec(ctx, host, plan, step)
	case "push":
		return r.executePush(ctx, host, plan, step)
	case "pull":
		return r.executePull(ctx, host, plan, step)
	case "mkdir":
		return r.executeMkdir(ctx, host, plan, step)
	case "wait":
		if plan.Check {
			return StepResult{Status: batch.StatusSkipped}
		}
		timer := time.NewTimer(step.Wait.Duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return failedStep(ctx.Err(), operation.StageTimeout)
		case <-timer.C:
			return StepResult{Status: batch.StatusOK}
		}
	case "confirm":
		if plan.Check {
			return StepResult{Status: batch.StatusSkipped}
		}
		return StepResult{Status: batch.StatusOK}
	default:
		return failedStep(fmt.Errorf("未知 deploy action"), operation.StageConfig)
	}
}

func (r Runner) executeExec(ctx context.Context, host config.Host, plan Plan, step Step) StepResult {
	if plan.Check && !step.CheckSafe {
		return StepResult{Status: batch.StatusSkipped}
	}
	command := step.Exec
	if step.Become {
		user := step.BecomeUser
		if user == "" {
			user = "root"
		}
		command = "sudo -n -u " + user + " -- sh -c " + shellQuote(step.Exec)
	}
	opResult := r.Executor.Exec(ctx, host, ops.ExecOptions{
		Command: command, ConnectTimeout: plan.ConnectTimeout.Duration,
		Stdout: r.Visible, Stderr: r.Visible,
	})
	rc, hasRemoteRC := remoteExitStatus(opResult.Err)
	failed := opResult.Err != nil
	if step.FailedWhen != nil && (opResult.Err == nil || hasRemoteRC) {
		failed = step.FailedWhen.Matches(rc)
	}
	if failed {
		result := failedStep(opResult.Err, opResult.Stage)
		if opResult.Err == nil {
			result = failedStep(fmt.Errorf("failed_when matched rc=%d", rc), operation.StageExecute)
		}
		result.Output, result.RC = opResult.Output, rc
		return result
	}
	status := batch.StatusOK
	if step.ChangedWhen != nil && step.ChangedWhen.Matches(rc) {
		status = batch.StatusChanged
	}
	return StepResult{Status: status, Output: opResult.Output, RC: rc}
}

func (r Runner) executePush(ctx context.Context, host config.Host, plan Plan, step Step) StepResult {
	action := step.Push
	opResult := r.Executor.Push(ctx, host, ops.TransferOptions{
		Direction: "push", Src: action.Src, Dest: action.Dest, Method: "auto",
		Overwrite: action.Overwrite, Backup: action.Backup, ValidateChecksum: action.ValidateChecksum(),
		Check: plan.Check, Diff: plan.Diff, ConnectTimeout: plan.ConnectTimeout.Duration,
	})
	return transferStepResult(opResult, plan.Check)
}

func (r Runner) executePull(ctx context.Context, host config.Host, plan Plan, step Step) StepResult {
	action := step.Pull
	destination, err := deployPullDestination(action.Dest, host.Alias, action.Src, action.Flat)
	if err != nil {
		return failedStep(err, operation.StageConfig)
	}
	opResult := r.Executor.Pull(ctx, host, ops.TransferOptions{
		Direction: "pull", Src: action.Src, Dest: destination, Method: "auto",
		Overwrite: action.Overwrite, Backup: action.Backup, ValidateChecksum: action.ValidateChecksum(),
		DestinationExact: true, Check: plan.Check, Diff: plan.Diff, ConnectTimeout: plan.ConnectTimeout.Duration,
	})
	return transferStepResult(opResult, plan.Check)
}

func (r Runner) executeMkdir(ctx context.Context, host config.Host, plan Plan, step Step) StepResult {
	action := step.Mkdir
	test := r.Executor.Exec(ctx, host, ops.ExecOptions{
		Command: "test -d " + shellQuote(action.Path), ConnectTimeout: plan.ConnectTimeout.Duration,
	})
	if test.Err == nil {
		return StepResult{Status: batch.StatusOK}
	}
	if _, hasRemoteRC := remoteExitStatus(test.Err); !hasRemoteRC {
		return failedStep(test.Err, test.Stage)
	}
	if plan.Check {
		return StepResult{Status: batch.StatusWouldChange}
	}
	mode := action.Mode
	if mode == "" {
		mode = "0755"
	}
	command := "mkdir -p -m " + mode + " -- " + shellQuote(action.Path)
	created := r.Executor.Exec(ctx, host, ops.ExecOptions{
		Command: command, ConnectTimeout: plan.ConnectTimeout.Duration,
	})
	if created.Err != nil {
		return failedStep(created.Err, created.Stage)
	}
	return StepResult{Status: batch.StatusChanged, Output: created.Output}
}

func transferStepResult(result ops.Result, check bool) StepResult {
	if result.Err != nil {
		failed := failedStep(result.Err, result.Stage)
		failed.Output = result.Output
		failed.Destination = result.Destination
		return failed
	}
	status := batch.StatusOK
	if result.Changed {
		status = batch.StatusChanged
		if check || result.WouldChange {
			status = batch.StatusWouldChange
		}
	}
	return StepResult{Status: status, Output: result.Output, Destination: result.Destination}
}

func deployPullDestination(root, alias, remote string, flat bool) (string, error) {
	clean := path.Clean(remote)
	if clean == "." || clean == "/" || hasParentComponent(clean) {
		return "", fmt.Errorf("pull 远程路径不安全: %s", remote)
	}
	name := path.Base(clean)
	if flat {
		return confinedDeployJoin(root, name)
	}
	relative := strings.TrimPrefix(clean, "/")
	return confinedDeployJoin(filepath.Join(root, alias), filepath.FromSlash(relative))
}

func confinedDeployJoin(root string, components ...string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(append([]string{absoluteRoot}, components...)...)
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absoluteRoot, absoluteTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("pull 目标路径逃逸: %s", target)
	}
	return target, nil
}

func hasParentComponent(value string) bool {
	for _, component := range strings.Split(filepath.ToSlash(value), "/") {
		if component == ".." {
			return true
		}
	}
	return false
}

func aggregateStatus(current, next batch.Status) batch.Status {
	if next == batch.StatusChanged {
		return batch.StatusChanged
	}
	if next == batch.StatusWouldChange && current != batch.StatusChanged {
		return batch.StatusWouldChange
	}
	return current
}

func failedStep(err error, stage operation.FailureStage) StepResult {
	if err == nil {
		err = fmt.Errorf("操作失败")
	}
	status := batch.StatusFailed
	switch stage {
	case operation.StageResolve, operation.StageNetwork, operation.StageJump, operation.StageTrust, operation.StageAuth:
		status = batch.StatusUnreachable
	}
	return StepResult{Status: status, Stage: stage, Reason: err.Error(), RC: exitStatus(err)}
}

func hostResultError(result HostResult) error {
	if result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable {
		return fmt.Errorf("%s", result.Reason)
	}
	return nil
}

func hostAddress(host config.Host) string {
	return fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port)
}

func exitStatus(err error) int {
	status, _ := remoteExitStatus(err)
	return status
}

func remoteExitStatus(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var status interface{ ExitStatus() int }
	if errors.As(err, &status) {
		return status.ExitStatus(), true
	}
	return 1, false
}

func retryCommand(plan Plan, alias string) string {
	return fmt.Sprintf("sshm deploy run %s --host %s --yes --file %s", shellQuote(plan.Profile), shellQuote(alias), shellQuote(plan.Config))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ops"
)

type Runner struct {
	Executor ops.Executor
	Visible  io.Writer
	Progress func(done, total int, result HostResult)
}

func (r Runner) Run(ctx context.Context, plan Plan) RunResult {
	result := RunResult{
		Profile: plan.Profile, Config: plan.Config, Mode: plan.Strategy.Mode,
		Targets: len(plan.Hosts), StartedAt: time.Now(), Results: make([]HostResult, len(plan.Hosts)),
	}
	if plan.Strategy.RunTimeout.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.Strategy.RunTimeout.Duration)
		defer cancel()
	}
	workers := min(plan.Strategy.MaxParallel, len(plan.Hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	done := 0
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				hostResult := r.runHost(ctx, plan.Hosts[index], plan)
				result.Results[index] = hostResult
				progressMu.Lock()
				done++
				if r.Progress != nil {
					r.Progress(done, len(plan.Hosts), hostResult)
				}
				progressMu.Unlock()
			}
		}()
	}
schedule:
	for index := range plan.Hosts {
		select {
		case <-ctx.Done():
			break schedule
		case jobs <- index:
		}
	}
	close(jobs)
	wg.Wait()
	result.Cancelled = ctx.Err() != nil
	for index, hostResult := range result.Results {
		if hostResult.HostAlias == "" {
			host := plan.Hosts[index]
			now := time.Now()
			result.Results[index] = HostResult{
				HostAlias: host.Alias, HostAddress: fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port),
				OK: false, Stage: operation.StageTimeout, Reason: ctx.Err().Error(), Suggestion: operation.Suggestion(operation.StageTimeout),
				RetryCommand: retryCommand(plan, host.Alias),
				StartedAt:    now, EndedAt: now,
			}
			hostResult = result.Results[index]
		}
		if hostResult.OK {
			result.OK++
		} else {
			result.Failed++
		}
	}
	result.EndedAt = time.Now()
	return result
}

func (r Runner) runHost(ctx context.Context, host config.Host, plan Plan) HostResult {
	hostResult := HostResult{
		HostAlias: host.Alias, HostAddress: fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port),
		StartedAt: time.Now(), OK: true,
	}
	if plan.Strategy.Mode == "visible" && r.Visible != nil {
		fmt.Fprintf(r.Visible, "\n========== %s (%s) ==========\n", host.Alias, host.Host)
	}
	for index, step := range plan.Steps {
		if ctx.Err() != nil {
			hostResult.OK = false
			hostResult.Stage = operation.StageTimeout
			hostResult.Reason = ctx.Err().Error()
			hostResult.Suggestion = operation.Suggestion(operation.StageTimeout)
			hostResult.RetryCommand = retryCommand(plan, host.Alias)
			break
		}
		stepResult := r.runStep(ctx, host, plan.Strategy, step, index)
		hostResult.Steps = append(hostResult.Steps, stepResult)
		if !stepResult.OK {
			hostResult.OK = false
			hostResult.FailedStep = stepResult.Name
			hostResult.Stage = stepResult.Stage
			hostResult.Reason = stepResult.Reason
			hostResult.Suggestion = operation.Suggestion(stepResult.Stage)
			hostResult.RetryCommand = retryCommand(plan, host.Alias)
			break
		}
	}
	hostResult.EndedAt = time.Now()
	if plan.Strategy.Mode == "visible" && r.Visible != nil {
		status := "OK"
		if !hostResult.OK {
			status = "FAILED"
		}
		fmt.Fprintf(r.Visible, "========== %s %s ==========\n", host.Alias, status)
	}
	return hostResult
}

func (r Runner) runStep(ctx context.Context, host config.Host, strategy Strategy, step Step, index int) StepResult {
	timeout := strategy.StepTimeout.Duration
	if step.Timeout.Duration > 0 {
		timeout = step.Timeout.Duration
	}
	stepCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	start := time.Now()
	attempts := 1
	result := r.executeStep(stepCtx, host, strategy, step)
	for !result.OK && step.Type == "copy" && attempts <= strategy.RetryCount && retryable(result.Stage, strategy.RetryOnStage) {
		attempts++
		result = r.executeStep(stepCtx, host, strategy, step)
	}
	stepResult := StepResult{
		Name: step.DisplayName(index), Type: step.Type, OK: result.OK,
		Output: result.Output, Method: result.Method, Destination: result.Destination,
		Attempts: attempts, DurationMS: time.Since(start).Milliseconds(),
	}
	if result.Err != nil {
		stepResult.Stage = result.Stage
		stepResult.Reason = result.Err.Error()
	}
	return stepResult
}

func (r Runner) executeStep(ctx context.Context, host config.Host, strategy Strategy, step Step) ops.Result {
	switch step.Type {
	case "exec":
		options := ops.ExecOptions{Command: step.Command, ConnectTimeout: strategy.ConnectTimeout.Duration}
		if strategy.Mode == "visible" {
			options.Stdout, options.Stderr = r.Visible, r.Visible
			if r.Visible != nil {
				fmt.Fprintf(r.Visible, "$ %s\n", step.Command)
			}
		}
		return r.Executor.Exec(ctx, host, options)
	case "copy":
		return r.Executor.Push(ctx, host, ops.TransferOptions{
			Direction: "push", Src: step.Src, Dest: step.Dest, Method: step.Method, Overwrite: step.Overwrite,
			ConnectTimeout: strategy.ConnectTimeout.Duration,
		})
	default:
		return ops.Result{Host: host, Stage: operation.StageConfig, Err: fmt.Errorf("未知 step type: %s", step.Type)}
	}
}

func retryable(stage operation.FailureStage, configured []string) bool {
	for _, value := range configured {
		if value == string(stage) {
			return true
		}
	}
	return false
}

func retryCommand(plan Plan, alias string) string {
	if plan.Config == "<command>" && len(plan.Steps) == 1 {
		step := plan.Steps[0]
		if step.Type == "exec" {
			return fmt.Sprintf("sshm deploy exec --host %s --cmd %s --yes", shellQuote(alias), shellQuote(step.Command))
		}
		command := fmt.Sprintf("sshm deploy copy --host %s --src %s --dest %s --method %s --yes",
			shellQuote(alias), shellQuote(step.Src), shellQuote(step.Dest), shellQuote(step.Method))
		if step.Overwrite {
			command += " --overwrite"
		}
		return command
	}
	return fmt.Sprintf("sshm deploy run %s --host %s --yes -f %s", shellQuote(plan.Profile), shellQuote(alias), shellQuote(plan.Config))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

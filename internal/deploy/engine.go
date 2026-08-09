package deploy

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
)

// Runner executes a resolved v3 plan.
type Runner struct {
	Executor       ops.Executor
	Visible        io.Writer
	Confirm        func(message string) error
	BecomePassword func(host config.Host) (string, bool)
	Event          func(PlayEvent)
}

// RunResult aggregates the outcome of a whole play.
type RunResult struct {
	Play       string        `json:"play"`
	Config     string        `json:"config"`
	Targets    int           `json:"targets"`
	Summary    batch.Summary `json:"summary"`
	Cancelled  bool          `json:"cancelled"`
	Check      bool          `json:"check"`
	StopReason string        `json:"stop_reason,omitempty"`
	StartedAt  time.Time     `json:"started_at"`
	EndedAt    time.Time     `json:"ended_at"`
	Hosts      []HostResult  `json:"hosts"`
	LogPath    string        `json:"log_path,omitempty"`
}

// HostResult is the per-host outcome of a play.
type HostResult struct {
	HostAlias   string                 `json:"host"`
	HostAddress string                 `json:"address"`
	Status      batch.Status           `json:"status"`
	FailedTask  string                 `json:"failed_task,omitempty"`
	Stage       operation.FailureStage `json:"stage,omitempty"`
	Reason      string                 `json:"reason,omitempty"`
	Suggestion  string                 `json:"suggestion,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	EndedAt     time.Time              `json:"ended_at"`
	Tasks       []TaskResult           `json:"tasks"`
}

// TaskResult is the per-host outcome of one task.
type TaskResult struct {
	Name       string       `json:"name"`
	Module     string       `json:"module,omitempty"`
	Status     batch.Status `json:"status"`
	Ignored    bool         `json:"ignored,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Output     string       `json:"output,omitempty"`
	RC         int          `json:"rc"`
	DurationMS int64        `json:"duration_ms"`
	Register   any          `json:"register,omitempty"`
}

func (r RunResult) ExitCode() int {
	switch {
	case r.Cancelled:
		return 130
	case r.Summary.Failed > 0 || (r.Summary.Skipped > 0 && !r.Check):
		return 1
	case r.Summary.Unreachable > 0:
		return 2
	default:
		return 0
	}
}

type hostState struct {
	host       config.Host
	registers  map[string]any
	facts      Vars
	status     batch.Status
	reason     string
	stage      operation.FailureStage
	failedTask string
	failed     bool
	tasks      []TaskResult
}

func newHostState(host config.Host) *hostState {
	return &hostState{
		host:      host,
		registers: map[string]any{},
		facts:     Vars{},
		status:    batch.StatusOK,
	}
}

// Run executes the plan and returns the aggregated result.
func (r Runner) Run(ctx context.Context, plan *Plan) RunResult {
	emitPlayStart(r, plan)
	result := RunResult{
		Play: plan.Name, Config: plan.Config, Targets: len(plan.Hosts),
		Check: plan.Check, StartedAt: time.Now(), Hosts: make([]HostResult, len(plan.Hosts)),
	}
	states := make(map[string]*hostState, len(plan.Hosts))
	for _, host := range plan.Hosts {
		states[host.Alias] = newHostState(host)
	}
	if plan.GatherFacts {
		gatherFactsForPlan(ctx, plan, states, r)
	}
	playState := NewPlayState()
	var cancelled bool
	var stopReason string
	if strategyOf(plan) == StrategyFree {
		cancelled, stopReason = r.runFree(ctx, plan, states, playState)
	} else {
		cancelled, stopReason = r.runLinear(ctx, plan, states, playState)
	}
	for index, host := range plan.Hosts {
		state := states[host.Alias]
		if state.status == batch.StatusOK && allTasksSkipped(state) {
			state.status = batch.StatusSkipped
		}
		now := time.Now()
		hostResult := HostResult{
			HostAlias: host.Alias, HostAddress: hostAddress(host),
			Status: state.status, FailedTask: state.failedTask,
			Stage: state.stage, Reason: state.reason,
			Suggestion: operation.Suggestion(state.stage),
			StartedAt:  now, EndedAt: now, Tasks: state.tasks,
		}
		if state.failed && hostResult.Suggestion == "" {
			hostResult.Suggestion = operation.Suggestion(operation.StageExecute)
		}
		result.Hosts[index] = hostResult
	}
	result.Summary = summarizeHosts(result.Hosts)
	result.Cancelled = cancelled
	result.StopReason = stopReason
	result.EndedAt = time.Now()
	emitPlayDone(r, plan, result)
	return result
}

// gatherFactsForPlan collects facts across hosts concurrently, bounded by the
// play's parallel setting, so large fleets are not serialized before tasks.
func gatherFactsForPlan(ctx context.Context, plan *Plan, states map[string]*hostState, r Runner) {
	parallel := plan.Batch.Parallel
	if parallel < 1 {
		parallel = 1
	}
	var wg sync.WaitGroup
	jobs := make(chan config.Host)
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				state := states[host.Alias]
				facts, err := gatherFacts(ctx, host, r.Executor, plan.ConnectTimeout.Duration, plan.FactsDir, !plan.Check)
				if err != nil {
					state.failed = true
					state.status = batch.StatusFailed
					state.stage = operation.StageExecute
					state.reason = "gather_facts 失败: " + err.Error()
					continue
				}
				state.facts = facts
			}
		}()
	}
	for _, host := range plan.Hosts {
		if ctx.Err() != nil {
			break
		}
		jobs <- host
	}
	close(jobs)
	wg.Wait()
}

func strategyOf(plan *Plan) string {
	if plan.Strategy == "" {
		return StrategyLinear
	}
	return plan.Strategy
}

func (r Runner) runLinear(ctx context.Context, plan *Plan, states map[string]*hostState, playState *PlayState) (bool, string) {
	for taskIndex, task := range plan.Tasks {
		runOnce := newRunOnceTracker()
		options := plan.Batch
		if task.RunOnce {
			options.Parallel = 1
			options.Serial = 1
		}
		batchResult, err := (batch.Runner{
			Options:     options,
			BeforeBatch: linearConfirmHook(r, plan, task),
			Progress: func(done, total int, item batch.Result) {
				r.emit(plan, taskIndex, task, item)
			},
		}).Run(ctx, plan.Hosts, func(ctx context.Context, host config.Host) batch.Result {
			state := states[host.Alias]
			if state.failed {
				state.tasks = append(state.tasks, TaskResult{
					Name: task.DisplayName(taskIndex), Module: task.Module,
					Status: batch.StatusSkipped, Reason: "主机已失败",
				})
				return batch.Result{Host: host, Status: batch.StatusSkipped, SkippedReason: "主机已失败"}
			}
			if task.RunOnce && !runOnce.TryClaim() {
				state.tasks = append(state.tasks, TaskResult{
					Name: task.DisplayName(taskIndex), Module: task.Module,
					Status: batch.StatusSkipped, Reason: "run_once 任务已执行",
				})
				return batch.Result{Host: host, Status: batch.StatusSkipped, SkippedReason: "run_once 任务已执行"}
			}
			tc := r.taskContext(ctx, plan, state, playState)
			taskResult := r.runTaskForHost(tc, task, taskIndex)
			state.tasks = append(state.tasks, taskResult)
			if isTaskFailure(taskResult) {
				state.failed = true
				state.status = taskResult.Status
				state.reason = taskResult.Reason
				state.stage = stageOf(taskResult)
				state.failedTask = taskResult.Name
			} else {
				state.status = aggregateHostStatus(state.status, taskResult.Status)
			}
			return batch.Result{
				Host: host, Status: taskResult.Status, Err: taskResultError(taskResult),
				Detail: taskResult.Reason, Value: taskResult,
			}
		})
		for _, item := range batchResult.Results {
			state := states[item.Host.Alias]
			if item.Status == batch.StatusSkipped && len(state.tasks) == taskIndex {
				state.tasks = append(state.tasks, TaskResult{
					Name: task.DisplayName(taskIndex), Module: task.Module,
					Status: batch.StatusSkipped, Reason: item.SkippedReason,
				})
				state.status = batch.StatusSkipped
				state.reason = item.SkippedReason
			}
		}
		if err != nil {
			return ctx.Err() != nil, err.Error()
		}
		if ctx.Err() != nil {
			return true, "用户中断或上下文取消"
		}
	}
	return false, ""
}

func (r Runner) runFree(ctx context.Context, plan *Plan, states map[string]*hostState, playState *PlayState) (bool, string) {
	runOnceFlags := make([]*runOnceTracker, len(plan.Tasks))
	for index := range plan.Tasks {
		runOnceFlags[index] = newRunOnceTracker()
	}
	batchResult, err := (batch.Runner{
		Options: plan.Batch,
		Progress: func(done, total int, item batch.Result) {
			r.emitFree(plan, item)
		},
	}).Run(ctx, plan.Hosts, func(ctx context.Context, host config.Host) batch.Result {
		state := states[host.Alias]
		tc := r.taskContext(ctx, plan, state, playState)
		tc.ConfirmLazy = true
		for taskIndex, task := range plan.Tasks {
			if state.failed {
				state.tasks = append(state.tasks, TaskResult{
					Name: task.DisplayName(taskIndex), Module: task.Module,
					Status: batch.StatusSkipped, Reason: "主机已失败",
				})
				continue
			}
			if task.RunOnce && !runOnceFlags[taskIndex].TryClaim() {
				state.tasks = append(state.tasks, TaskResult{
					Name: task.DisplayName(taskIndex), Module: task.Module,
					Status: batch.StatusSkipped, Reason: "run_once 任务已执行",
				})
				continue
			}
			taskResult := r.runTaskForHost(tc, task, taskIndex)
			state.tasks = append(state.tasks, taskResult)
			if isTaskFailure(taskResult) {
				state.failed = true
				state.status = taskResult.Status
				state.reason = taskResult.Reason
				state.stage = stageOf(taskResult)
				state.failedTask = taskResult.Name
				break
			} else {
				state.status = aggregateHostStatus(state.status, taskResult.Status)
			}
		}
		return batch.Result{
			Host: host, Status: state.status, Err: hostResultError(state),
			Detail: state.reason, Value: state,
		}
	})
	for _, item := range batchResult.Results {
		state := states[item.Host.Alias]
		if item.Status == batch.StatusSkipped && len(state.tasks) == 0 {
			state.status = batch.StatusSkipped
			state.reason = item.SkippedReason
		}
	}
	if err != nil {
		return ctx.Err() != nil, err.Error()
	}
	if ctx.Err() != nil {
		return true, "用户中断或上下文取消"
	}
	return false, ""
}

// linearConfirmHook gates each serial batch of a task that declares confirm,
// mirroring the former v2 confirm step semantics.
func linearConfirmHook(r Runner, plan *Plan, task Task) func(start, end, total int) error {
	if task.Confirm == "" || plan.Check {
		return nil
	}
	message := task.Confirm
	return func(start, end, total int) error {
		if r.Confirm == nil {
			return fmt.Errorf("deploy confirm 需要交互终端: %s", message)
		}
		return r.Confirm(message)
	}
}

func (r Runner) taskContext(ctx context.Context, plan *Plan, state *hostState, playState *PlayState) TaskContext {
	tc := TaskContext{
		Ctx:            ctx,
		Host:           state.host,
		Vars:           cloneVars(plan.Vars),
		Registers:      state.registers,
		Facts:          state.facts,
		Check:          plan.Check,
		Diff:           plan.Diff,
		Confirm:        r.Confirm,
		Timeout:        plan.Timeout.Duration,
		ConnectTimeout: plan.ConnectTimeout.Duration,
		Executor:       r.Executor,
		Visible:        r.Visible,
		PlayState:      playState,
	}
	if r.BecomePassword != nil {
		if password, ok := r.BecomePassword(state.host); ok {
			tc.BecomePassword = password
			tc.HasBecomePassword = true
		}
	}
	return tc
}

func (r Runner) runTaskForHost(tc TaskContext, task Task, index int) TaskResult {
	start := time.Now()
	result := r.executeTask(tc, task)
	return TaskResult{
		Name:       task.DisplayName(index),
		Module:     task.Module,
		Status:     result.Status,
		Ignored:    task.IgnoreErrors && isModuleFailure(result),
		Reason:     resultReason(result),
		Output:     result.Output,
		RC:         result.RC,
		DurationMS: time.Since(start).Milliseconds(),
		Register:   result.Register,
	}
}

func (r Runner) executeTask(tc TaskContext, task Task) ModuleResult {
	if task.BaseDir != "" {
		tc.BaseDir = task.BaseDir
	}
	if task.ProjectRoot != "" {
		tc.ProjectRoot = task.ProjectRoot
	}
	if task.When != "" && len(task.Loop) == 0 {
		env := taskEnv(tc)
		matched, err := EvalWhen(task.When, env)
		if err != nil {
			return failedModule(err, operation.StageConfig)
		}
		if !matched {
			return ModuleResult{Status: batch.StatusSkipped}
		}
	}
	if task.Confirm != "" && tc.ConfirmLazy && !tc.Check {
		if err := tc.PlayState.ConfirmOnce(task.Confirm, tc.Confirm); err != nil {
			return failedModule(err, operation.StageConfig)
		}
	}
	if len(task.Block) > 0 {
		return r.executeBlock(tc, task)
	}
	if task.Module == "" {
		return failedModule(fmt.Errorf("task 没有可执行内容"), operation.StageConfig)
	}
	module, ok := Lookup(task.Module)
	if !ok {
		return failedModule(fmt.Errorf("未知模块 %q", task.Module), operation.StageConfig)
	}
	var result ModuleResult
	if len(task.Loop) > 0 {
		result = r.executeLoop(tc, task, module)
	} else {
		result = r.executeOnce(tc, task, module, nil, -1)
	}
	result = r.applyOverrides(tc, task, result)
	if task.Register != "" {
		tc.Registers[task.Register] = registerValue(result)
	}
	return result
}

func (r Runner) executeOnce(tc TaskContext, task Task, module Module, loopItem any, loopIndex int) ModuleResult {
	vars := cloneVars(tc.Vars)
	for key, value := range tc.Facts {
		if _, exists := vars[key]; !exists {
			vars[key] = value
		}
		vars["ansible_"+key] = value
	}
	if loopItem != nil {
		vars["item"] = loopItem
		vars["loop_index"] = loopIndex
	}
	if err := validateCommandTemplateBoundary(task.Module, task.Args); err != nil {
		return failedModule(err, operation.StageConfig)
	}
	args, err := renderArgs(task.Args, vars)
	if err != nil {
		return failedModule(err, operation.StageConfig)
	}
	decoded, err := module.DecodeArgs(args)
	if err != nil {
		return failedModule(err, operation.StageConfig)
	}
	execCtx := tc.Ctx
	cancel := func() {}
	if !moduleOwnsTimeout(task.Module) {
		execCtx, cancel = context.WithTimeout(tc.Ctx, tc.Timeout)
	}
	defer cancel()
	runCtx := tc
	runCtx.Ctx = execCtx
	runCtx.Vars = vars
	runCtx.LoopItem = loopItem
	runCtx.LoopIndex = loopIndex
	runCtx.CheckSafe = task.CheckSafe
	if task.Become {
		runCtx.Become = true
		runCtx.BecomeUser = task.BecomeUser
	}
	if len(task.Env) > 0 {
		runCtx.Env = task.Env
	}
	return module.Run(runCtx, decoded)
}

// moduleOwnsTimeout reports whether a module bounds its own duration (sleep,
// wait_for) and must not be truncated by the generic task-level timeout.
func moduleOwnsTimeout(name string) bool {
	switch name {
	case "sleep", "wait_for":
		return true
	default:
		return false
	}
}

func (r Runner) executeLoop(tc TaskContext, task Task, module Module) ModuleResult {
	overall := ModuleResult{Status: batch.StatusOK}
	executed := false
	var registers []any
	for index, item := range task.Loop {
		if task.When != "" {
			env := taskEnv(tc)
			env["item"] = item
			env["loop_index"] = index
			matched, err := EvalWhen(task.When, env)
			if err != nil {
				return failedModule(err, operation.StageConfig)
			}
			if !matched {
				continue
			}
		}
		executed = true
		result := r.executeOnce(tc, task, module, item, index)
		registers = append(registers, result.Register)
		overall.Output += result.Output
		if result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable {
			overall = result
			break
		}
		if result.WouldChange {
			overall.WouldChange = true
			overall.Status = batch.StatusWouldChange
		} else if result.Changed {
			overall.Changed = true
			overall.Status = batch.StatusChanged
		}
	}
	if !executed {
		return ModuleResult{Status: batch.StatusSkipped}
	}
	overall.Register = registers
	return overall
}

func (r Runner) executeBlock(tc TaskContext, task Task) ModuleResult {
	outputs := strings.Builder{}
	runChildren := func(children []Task) (ModuleResult, batch.Status) {
		status := batch.StatusOK
		changed := false
		wouldChange := false
		var lastResult ModuleResult
		for _, child := range children {
			result := r.executeTask(tc, child)
			if result.Status == batch.StatusSkipped {
				continue
			}
			lastResult = result
			if result.Output != "" {
				outputs.WriteString(result.Output)
			}
			if result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable {
				return result, result.Status
			}
			if result.Changed {
				changed = true
			}
			if result.WouldChange {
				wouldChange = true
			}
		}
		if changed {
			status = batch.StatusChanged
		} else if wouldChange {
			status = batch.StatusWouldChange
		}
		return lastResult, status
	}

	last, blockStatus := runChildren(task.Block)
	if (blockStatus == batch.StatusFailed || blockStatus == batch.StatusUnreachable) && len(task.Rescue) > 0 {
		last, blockStatus = runChildren(task.Rescue)
	}
	if len(task.Always) > 0 {
		for _, child := range task.Always {
			result := r.executeTask(tc, child)
			if result.Status == batch.StatusSkipped {
				continue
			}
			last = result
			if result.Output != "" {
				outputs.WriteString(result.Output)
			}
			if result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable {
				blockStatus = result.Status
				break
			}
		}
	}
	last.Output = outputs.String()
	last.Status = blockStatus
	last.Changed = blockStatus == batch.StatusChanged
	last.WouldChange = blockStatus == batch.StatusWouldChange
	return last
}

// applyOverrides applies task-level failed_when/changed_when and ignore_errors.
func (r Runner) applyOverrides(tc TaskContext, task Task, result ModuleResult) ModuleResult {
	if task.FailedWhen != nil {
		failed := task.FailedWhen.Matches(result.RC)
		if failed && result.Status != batch.StatusFailed && result.Status != batch.StatusUnreachable {
			return ModuleResult{
				Status: batch.StatusFailed, RC: result.RC, Output: result.Output,
				Err:   fmt.Errorf("failed_when 匹配 rc=%d", result.RC),
				Stage: operation.StageExecute,
			}
		}
		if !failed && result.Status == batch.StatusFailed {
			result.Status = batch.StatusOK
			result.Err = nil
		}
	}
	if task.ChangedWhen != nil && task.ChangedWhen.Matches(result.RC) {
		result.Changed = true
		result.Status = batch.StatusChanged
	}
	if task.IgnoreErrors && (result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable) {
		result.Status = batch.StatusOK
		result.Err = nil
		result.Output += "（已忽略错误）"
	}
	return result
}

func taskEnv(tc TaskContext) map[string]any {
	env := map[string]any{}
	for key, value := range tc.Vars {
		env[key] = value
	}
	for key, value := range tc.Facts {
		env[key] = value
		env["ansible_"+key] = value
	}
	for key, value := range tc.Registers {
		env[key] = value
	}
	if tc.LoopItem != nil {
		env["item"] = tc.LoopItem
		env["loop_index"] = tc.LoopIndex
	}
	return env
}

func registerValue(result ModuleResult) any {
	if result.Register != nil {
		return result.Register
	}
	out := map[string]any{
		"changed": result.Changed || result.Status == batch.StatusChanged,
		"rc":      result.RC,
		"output":  result.Output,
		"status":  string(result.Status),
	}
	if result.Err != nil {
		out["failed"] = true
		out["reason"] = result.Err.Error()
	}
	return out
}

func isTaskFailure(result TaskResult) bool {
	return result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable
}

func isModuleFailure(result ModuleResult) bool {
	return result.Status == batch.StatusFailed || result.Status == batch.StatusUnreachable
}

func aggregateHostStatus(current batch.Status, task batch.Status) batch.Status {
	switch task {
	case batch.StatusChanged:
		return batch.StatusChanged
	case batch.StatusWouldChange:
		if current != batch.StatusChanged {
			return batch.StatusWouldChange
		}
	}
	return current
}

func allTasksSkipped(state *hostState) bool {
	if len(state.tasks) == 0 {
		return false
	}
	for _, task := range state.tasks {
		if task.Status != batch.StatusSkipped {
			return false
		}
	}
	return true
}

func stageOf(result TaskResult) operation.FailureStage {
	if result.Status == batch.StatusUnreachable {
		return operation.StageNetwork
	}
	return operation.StageExecute
}

func taskResultError(result TaskResult) error {
	if isTaskFailure(result) {
		return fmt.Errorf("%s", result.Reason)
	}
	return nil
}

func hostResultError(state *hostState) error {
	if state.failed {
		return fmt.Errorf("%s", state.reason)
	}
	return nil
}

func resultReason(result ModuleResult) string {
	switch result.Status {
	case batch.StatusSkipped:
		if result.SkipReason != "" {
			return result.SkipReason
		}
		return "when 条件不满足"
	case batch.StatusFailed, batch.StatusUnreachable:
		if result.Err != nil {
			return result.Err.Error()
		}
		return "任务失败"
	default:
		return ""
	}
}

func hostAddress(host config.Host) string {
	return fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port)
}

func summarizeHosts(hosts []HostResult) batch.Summary {
	var summary batch.Summary
	for _, host := range hosts {
		switch host.Status {
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

type runOnceTracker struct {
	mu   sync.Mutex
	used bool
}

func newRunOnceTracker() *runOnceTracker {
	return &runOnceTracker{}
}

func (t *runOnceTracker) TryClaim() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.used {
		return false
	}
	t.used = true
	return true
}

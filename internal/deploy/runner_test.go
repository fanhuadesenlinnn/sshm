package deploy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

type fakeExecutor struct {
	mu         sync.Mutex
	calls      []string
	active     int
	maxActive  int
	execResult map[string]ops.Result
}

func (f *fakeExecutor) Exec(_ context.Context, host config.Host, options ops.ExecOptions) ops.Result {
	f.begin(host.Alias + ":exec:" + options.Command)
	defer f.end()
	time.Sleep(3 * time.Millisecond)
	if result, ok := f.execResult[options.Command]; ok {
		result.Host = host
		return result
	}
	return ops.Result{Host: host, OK: true, Output: "ok"}
}

func (f *fakeExecutor) Push(_ context.Context, host config.Host, options ops.TransferOptions) ops.Result {
	f.begin(host.Alias + ":push")
	defer f.end()
	return ops.Result{Host: host, OK: true, Changed: true, WouldChange: options.Check}
}

func (f *fakeExecutor) Pull(_ context.Context, host config.Host, options ops.TransferOptions) ops.Result {
	f.begin(host.Alias + ":pull")
	defer f.end()
	return ops.Result{Host: host, OK: true, Changed: true, WouldChange: options.Check, Destination: options.Dest}
}

func (f *fakeExecutor) begin(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
}

func (f *fakeExecutor) end() {
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
}

func testPlan(hosts ...string) Plan {
	plan := Plan{
		Profile: "test", Config: "/tmp/deploy.yaml",
		Batch: batch.Options{Parallel: 2}, Timeout: Duration{Duration: time.Second},
		ConnectTimeout: Duration{Duration: time.Second},
	}
	for _, alias := range hosts {
		plan.Hosts = append(plan.Hosts, config.Host{Alias: alias})
	}
	return plan
}

func TestRunnerCheckDoesNotWriteAndReportsExpectedStatuses(t *testing.T) {
	executor := &fakeExecutor{execResult: map[string]ops.Result{
		"test -d '/tmp/new'": {Stage: operation.StageExecute, Err: exitCodeError(1)},
	}}
	plan := testPlan("one")
	plan.Check = true
	wait := Duration{Duration: time.Second}
	plan.Steps = []Step{
		{Name: "push", Push: &PushAction{Src: "/tmp/src", Dest: "/tmp/dest"}},
		{Name: "mkdir", Mkdir: &MkdirAction{Path: "/tmp/new"}},
		{Name: "unsafe", Exec: "unsafe"},
		{Name: "safe", Exec: "safe", CheckSafe: true},
		{Name: "wait", Wait: &wait},
	}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	statuses := []batch.Status{}
	for _, step := range result.Results[0].Steps {
		statuses = append(statuses, step.Status)
	}
	want := []batch.Status{batch.StatusWouldChange, batch.StatusWouldChange, batch.StatusSkipped, batch.StatusOK, batch.StatusSkipped}
	if strings.TrimSpace(strings.Join(statusStrings(statuses), ",")) != strings.Join(statusStrings(want), ",") {
		t.Fatalf("statuses = %v", statuses)
	}
	for _, call := range executor.calls {
		if strings.Contains(call, "mkdir -p") || strings.Contains(call, "unsafe") {
			t.Fatalf("check executed mutating call: %s", call)
		}
	}
}

func TestRunnerHandlersOnceAndIgnoreError(t *testing.T) {
	executor := &fakeExecutor{execResult: map[string]ops.Result{
		"fail": {Stage: operation.StageExecute, Err: errors.New("failed")},
	}}
	plan := testPlan("one")
	plan.Steps = []Step{
		{Name: "ignored", Exec: "fail", IgnoreError: true},
		{Name: "one", Push: &PushAction{Src: "/tmp/a", Dest: "/tmp/a"}, Notify: []string{"restart"}},
		{Name: "two", Push: &PushAction{Src: "/tmp/b", Dest: "/tmp/b"}, Notify: []string{"restart"}},
	}
	plan.Handlers = []Step{{Name: "restart", Exec: "restart"}}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	host := result.Results[0]
	if host.Status != batch.StatusChanged || !host.Steps[0].Ignored || len(host.Steps) != 4 {
		t.Fatalf("host result = %+v", host)
	}
	count := 0
	for _, call := range executor.calls {
		if strings.Contains(call, ":exec:restart") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("handler calls = %d, calls=%v", count, executor.calls)
	}
}

func TestRunnerConditionsBecomeAndFailureThreshold(t *testing.T) {
	executor := &fakeExecutor{execResult: map[string]ops.Result{
		"rc10": {OK: true, Err: exitCodeError(10)},
	}}
	plan := testPlan("one", "two", "three")
	plan.Batch = batch.Options{Parallel: 2, Serial: 2, FailFast: true}
	plan.Steps = []Step{{
		Name: "condition", Exec: "rc10", IgnoreError: true,
		FailedWhen: &Condition{RCIn: []int{1}}, ChangedWhen: &Condition{RCIn: []int{10}},
	}}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 3 || result.Summary.Failed != 0 || executor.maxActive != 2 {
		t.Fatalf("result=%+v maxActive=%d", result.Summary, executor.maxActive)
	}

	executor = &fakeExecutor{}
	plan = testPlan("one")
	plan.Steps = []Step{{Exec: "echo 'ok'", Become: true, BecomeUser: "deploy"}}
	(Runner{Executor: executor}).Run(context.Background(), plan)
	if len(executor.calls) != 1 || !strings.Contains(executor.calls[0], `sudo -n -u deploy -- sh -c 'echo '"'"'ok'"'"''`) {
		t.Fatalf("become call = %v", executor.calls)
	}
}

func TestFailedWhenCannotHideConnectionFailure(t *testing.T) {
	executor := &fakeExecutor{execResult: map[string]ops.Result{
		"connect": {Stage: operation.StageAuth, Err: errors.New("authentication failed")},
	}}
	plan := testPlan("one")
	plan.Steps = []Step{{Exec: "connect", FailedWhen: &Condition{RCIn: []int{2}}}}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Unreachable != 1 || result.Results[0].Status != batch.StatusUnreachable {
		t.Fatalf("result = %+v", result)
	}

	executor = &fakeExecutor{execResult: map[string]ops.Result{
		"connect": {Stage: operation.StageCredential, Err: errors.New("missing credentials")},
	}}
	result = (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Unreachable != 1 || result.Results[0].Status != batch.StatusUnreachable {
		t.Fatalf("credential failure should be unreachable: %+v", result)
	}
}

func TestMkdirCheckCannotHideConnectionFailure(t *testing.T) {
	plan := testPlan("one")
	plan.Check = true
	plan.Steps = []Step{{Mkdir: &MkdirAction{Path: "/opt/app"}}}
	executor := &fakeExecutor{execResult: map[string]ops.Result{
		"test -d '/opt/app'": {
			Err:   errors.New("connection refused"),
			Stage: operation.StageNetwork,
		},
	}}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Unreachable != 1 || result.Results[0].Status != batch.StatusUnreachable {
		t.Fatalf("mkdir connection failure was hidden: %+v", result)
	}
}

func TestRunnerConfirmOncePerSerialBatch(t *testing.T) {
	plan := testPlan("one", "two", "three")
	plan.Batch = batch.Options{Parallel: 2, Serial: 2}
	plan.Steps = []Step{{Name: "gate", Confirm: "继续？"}, {Exec: "hostname"}}
	var prompts []string
	result := (Runner{
		Executor: &fakeExecutor{},
		Confirm: func(message string) error {
			prompts = append(prompts, message)
			return nil
		},
	}).Run(context.Background(), plan)
	if len(prompts) != 2 || result.Summary.OK != 3 {
		t.Fatalf("prompts=%v summary=%+v", prompts, result.Summary)
	}
}

func TestRunnerRefusedLaterBatchKeepsCompletedResults(t *testing.T) {
	plan := testPlan("one", "two", "three")
	plan.Batch = batch.Options{Parallel: 2, Serial: 2}
	plan.Steps = []Step{{Confirm: "继续？"}, {Exec: "hostname"}}
	prompts := 0
	result := (Runner{
		Executor: &fakeExecutor{},
		Confirm: func(string) error {
			prompts++
			if prompts == 2 {
				return errors.New("refused")
			}
			return nil
		},
	}).Run(context.Background(), plan)
	if result.Summary.OK != 2 || result.Summary.Failed != 1 || result.Summary.Skipped != 0 {
		t.Fatalf("summary=%+v results=%+v", result.Summary, result.Results)
	}
}

func TestRunnerRejectsFlatPullConflictBeforeExecution(t *testing.T) {
	executor := &fakeExecutor{}
	plan := testPlan("one", "two")
	plan.Steps = []Step{{Name: "pull", Pull: &PullAction{Src: "/etc/hosts", Dest: t.TempDir(), Flat: true}}}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Failed != 2 || len(executor.calls) != 0 || !strings.Contains(result.StopReason, "冲突") {
		t.Fatalf("result=%+v calls=%v", result, executor.calls)
	}
}

func statusStrings(statuses []batch.Status) []string {
	out := make([]string, len(statuses))
	for index, status := range statuses {
		out[index] = string(status)
	}
	return out
}

type exitCodeError int

func (e exitCodeError) Error() string   { return "exit" }
func (e exitCodeError) ExitStatus() int { return int(e) }

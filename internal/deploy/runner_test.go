package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ops"
)

type fakeExecutor struct {
	mu         sync.Mutex
	calls      []string
	active     int
	maxActive  int
	copyFails  int
	execFailed map[string]bool
}

func (f *fakeExecutor) Exec(_ context.Context, host config.Host, options ops.ExecOptions) ops.Result {
	f.begin(host.Alias + ":exec:" + options.Command)
	defer f.end()
	time.Sleep(5 * time.Millisecond)
	if f.execFailed[host.Alias] {
		return ops.Result{Host: host, Stage: operation.StageExecute, Err: errors.New("failed")}
	}
	return ops.Result{Host: host, OK: true, Output: "ok"}
}

func (f *fakeExecutor) Push(_ context.Context, host config.Host, _ ops.TransferOptions) ops.Result {
	f.begin(host.Alias + ":copy")
	defer f.end()
	f.mu.Lock()
	fail := f.copyFails > 0
	if fail {
		f.copyFails--
	}
	f.mu.Unlock()
	if fail {
		return ops.Result{Host: host, Stage: operation.StageTransfer, Err: errors.New("retry")}
	}
	return ops.Result{Host: host, OK: true, Method: "sftp"}
}

func (f *fakeExecutor) Pull(context.Context, config.Host, ops.TransferOptions) ops.Result {
	panic("unexpected pull")
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

func TestRunnerStopsFailedHostAndContinuesOthers(t *testing.T) {
	executor := &fakeExecutor{execFailed: map[string]bool{"one": true}}
	plan := Plan{
		Profile: "test", Hosts: []config.Host{{Alias: "one"}, {Alias: "two"}},
		Strategy: Strategy{Mode: "hidden", MaxParallel: 2, StepTimeout: Duration{Duration: time.Second}},
		Steps:    []Step{{Name: "first", Type: "exec", Command: "first"}, {Name: "second", Type: "exec", Command: "second"}},
	}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.OK != 1 || result.Failed != 1 || len(result.Results[0].Steps) != 1 || len(result.Results[1].Steps) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if executor.maxActive < 2 {
		t.Fatalf("max concurrency = %d", executor.maxActive)
	}
}

func TestRunnerRetriesCopyButNeverExec(t *testing.T) {
	executor := &fakeExecutor{copyFails: 1, execFailed: map[string]bool{"one": true}}
	plan := Plan{
		Profile: "retry", Hosts: []config.Host{{Alias: "one"}},
		Strategy: Strategy{
			Mode: "hidden", MaxParallel: 1, StepTimeout: Duration{Duration: time.Second},
			RetryCount: 2, RetryOnStage: []string{"transfer"},
		},
		Steps: []Step{{Type: "copy"}, {Type: "exec", Command: "unsafe"}},
	}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Results[0].Steps[0].Attempts != 2 || result.Results[0].Steps[1].Attempts != 1 {
		t.Fatalf("attempts = %+v", result.Results[0].Steps)
	}
}

func TestVisibleStrategyRequiresSerialAndJSONIsValid(t *testing.T) {
	if err := validateStrategy(Strategy{Mode: "visible", MaxParallel: 2}); err == nil {
		t.Fatal("visible parallel should fail")
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, RunResult{Profile: "test", OK: 1}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerCancellationKeepsAllHostResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := Plan{
		Profile: "cancel", Hosts: []config.Host{{Alias: "one"}, {Alias: "two"}},
		Strategy: Strategy{Mode: "hidden", MaxParallel: 1, StepTimeout: Duration{Duration: time.Second}},
		Steps:    []Step{{Type: "exec", Command: "hostname"}},
	}
	result := (Runner{Executor: &fakeExecutor{execFailed: map[string]bool{}}}).Run(ctx, plan)
	if !result.Cancelled || result.Failed != 2 {
		t.Fatalf("result = %+v", result)
	}
	for _, host := range result.Results {
		if host.HostAlias == "" || host.Stage != operation.StageTimeout {
			t.Fatalf("cancelled host = %+v", host)
		}
	}
}

func TestRetryCommandHandlesProfileAndOneShotPlans(t *testing.T) {
	profile := Plan{Profile: "install app", Config: "/tmp/deploy's.yaml"}
	if got := retryCommand(profile, "web one"); got != "sshm deploy run 'install app' --host 'web one' --yes -f '/tmp/deploy'\"'\"'s.yaml'" {
		t.Fatalf("profile retry = %q", got)
	}
	oneShot := Plan{Config: "<command>", Steps: []Step{{Type: "exec", Command: "echo 'ok'"}}}
	if got := retryCommand(oneShot, "one"); got != "sshm deploy exec --host 'one' --cmd 'echo '\"'\"'ok'\"'\"'' --yes" {
		t.Fatalf("one-shot retry = %q", got)
	}
}

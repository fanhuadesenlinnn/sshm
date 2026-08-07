package deployv3

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

type fakeExecutor struct {
	mu       sync.Mutex
	calls    []string
	execFail map[string]error
	statInfo map[string]ops.RemoteFileInfo
}

func (f *fakeExecutor) Exec(_ context.Context, host config.Host, options ops.ExecOptions) ops.Result {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":exec:"+options.Command)
	f.mu.Unlock()
	if err, ok := f.execFail[options.Command]; ok {
		return ops.Result{Host: host, Stage: operation.StageExecute, Err: err}
	}
	return ops.Result{Host: host, OK: true, Output: "ok"}
}

func (f *fakeExecutor) Push(_ context.Context, host config.Host, options ops.TransferOptions) ops.Result {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":push:"+options.Src+"->"+options.Dest)
	f.mu.Unlock()
	return ops.Result{Host: host, OK: true, Changed: true, WouldChange: options.Check}
}

func (f *fakeExecutor) Pull(_ context.Context, host config.Host, options ops.TransferOptions) ops.Result {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":pull:"+options.Src+"->"+options.Dest)
	f.mu.Unlock()
	return ops.Result{Host: host, OK: true, Changed: true, WouldChange: options.Check, Destination: options.Dest}
}

func (f *fakeExecutor) Stat(_ context.Context, host config.Host, path string, _ time.Duration) (ops.RemoteFileInfo, error) {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":stat:"+path)
	f.mu.Unlock()
	info, ok := f.statInfo[path]
	if ok {
		return info, nil
	}
	return ops.RemoteFileInfo{}, nil
}

func (f *fakeExecutor) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func planFor(t *testing.T, tasks []Task, hosts []config.Host) *Plan {
	t.Helper()
	play := Play{Name: "test", Hosts: deploy.TargetSelector{Hosts: []string{"h1", "h2"}}, Tasks: tasks}
	catalog := &Catalog{ByName: map[string]Play{"test": play}}
	plan, err := BuildPlan(catalog, "test", hosts, Overrides{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return plan
}

func testHosts() []config.Host {
	return []config.Host{
		{Alias: "h1", Host: "10.0.0.1", User: "root", Port: 22},
		{Alias: "h2", Host: "10.0.0.2", User: "root", Port: 22},
	}
}

func commandTask(name, cmd string) Task {
	return Task{
		Name: name, Module: "command",
		Args: argsNode(map[string]any{"cmd": cmd}),
	}
}

func TestLinearStrategyRunsTaskMajor(t *testing.T) {
	executor := &fakeExecutor{}
	plan := planFor(t, []Task{
		commandTask("first", "echo one"),
		commandTask("second", "echo two"),
	}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	calls := executor.callLog()
	first := -1
	second := -1
	for index, call := range calls {
		switch {
		case strings.Contains(call, "echo one"):
			first = index
		case strings.Contains(call, "echo two"):
			if second < 0 {
				second = index
			}
		}
	}
	if first < 0 || second < 0 || second < first {
		t.Fatalf("linear 顺序错误: %v", calls)
	}
}

func TestRegisterAndWhen(t *testing.T) {
	executor := &fakeExecutor{}
	first := commandTask("upload", "echo upload")
	first.Register = "upload"
	first.ChangedWhen = &deploy.Condition{RCIn: []int{0}}
	second := commandTask("restart", "echo restart")
	second.When = "upload.changed"
	plan := planFor(t, []Task{first, second}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	calls := executor.callLog()
	restartCount := 0
	for _, call := range calls {
		if strings.Contains(call, "echo restart") {
			restartCount++
		}
	}
	if restartCount != 2 {
		t.Fatalf("restart 执行次数 = %d, want 2; calls=%v", restartCount, calls)
	}
}

func TestWhenFalseSkipsTask(t *testing.T) {
	executor := &fakeExecutor{}
	first := commandTask("upload", "echo upload")
	first.Register = "upload"
	second := commandTask("restart", "echo restart")
	second.When = "upload.changed"
	plan := planFor(t, []Task{first, second}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	calls := executor.callLog()
	for _, call := range calls {
		if strings.Contains(call, "echo restart") {
			t.Fatalf("when=false 不应执行 restart: %v", calls)
		}
	}
	if result.Summary.OK != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

func TestBlockRescueAlways(t *testing.T) {
	executor := &fakeExecutor{
		execFail: map[string]error{"/bin/false": &fakeExitError{code: 1}},
	}
	blockTask := Task{
		Name: "publish",
		Block: []Task{
			commandTask("deploy", "/bin/true"),
			commandTask("fail-step", "/bin/false"),
		},
		Rescue: []Task{
			commandTask("rollback", "echo rollback"),
		},
		Always: []Task{
			commandTask("cleanup", "echo cleanup"),
		},
	}
	plan := planFor(t, []Task{blockTask}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Failed != 0 || result.Summary.OK != 2 {
		t.Fatalf("rescue 应使主机恢复: %+v", result.Summary)
	}
	calls := executor.callLog()
	rollback := 0
	cleanup := 0
	for _, call := range calls {
		if strings.Contains(call, "echo rollback") {
			rollback++
		}
		if strings.Contains(call, "echo cleanup") {
			cleanup++
		}
	}
	if rollback != 2 || cleanup != 2 {
		t.Fatalf("rollback=%d cleanup=%d; calls=%v", rollback, cleanup, calls)
	}
}

func TestRunOnceExecutesSingleHost(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("generate-key", "echo key")
	task.RunOnce = true
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	calls := executor.callLog()
	count := 0
	for _, call := range calls {
		if strings.Contains(call, "echo key") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("run_once 执行次数 = %d, want 1; calls=%v", count, calls)
	}
	if result.Summary.Skipped != 1 {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

func TestLoopRendersItem(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("render", "echo {{ item }}")
	task.Loop = []string{"a", "b"}
	plan := planFor(t, []Task{task}, testHosts())
	(Runner{Executor: executor}).Run(context.Background(), plan)
	calls := executor.callLog()
	for _, want := range []string{"echo a", "echo b"} {
		count := 0
		for _, call := range calls {
			if strings.Contains(call, want) {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("%q 执行次数 = %d, want 2; calls=%v", want, count, calls)
		}
	}
}

func TestCheckModeCommandSkipped(t *testing.T) {
	executor := &fakeExecutor{}
	plan := planFor(t, []Task{commandTask("unsafe", "echo boom")}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Skipped != 2 {
		t.Fatalf("check 模式不安全命令应跳过: %+v", result.Summary)
	}
}

func TestTaskBaseDirResolvesCopySrc(t *testing.T) {
	executor := &fakeExecutor{}
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "dist", "app.tar.gz"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	task := Task{
		Name: "upload", Module: "copy",
		Args: argsNode(map[string]any{"src": "./dist/app.tar.gz", "dest": "/tmp/app.tar.gz"}),
	}
	task.BaseDir = base
	plan := planFor(t, []Task{task}, testHosts())
	(Runner{Executor: executor}).Run(context.Background(), plan)
	calls := executor.callLog()
	found := false
	for _, call := range calls {
		if strings.Contains(call, filepath.Join(base, "dist", "app.tar.gz")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("copy src 未按 BaseDir 解析: %v", calls)
	}
}

func TestFactsAvailableInTaskArgs(t *testing.T) {
	executor := &fakeExecutor{}
	task := Task{
		Name: "debug-facts", Module: "debug",
		Args: argsNode(map[string]any{"msg": "os={{ os_family }} arch={{ arch }}"}),
	}
	play := Play{Name: "test", Hosts: deploy.TargetSelector{Hosts: []string{"h1"}}, Tasks: []Task{task}}
	catalog := &Catalog{ByName: map[string]Play{"test": play}}
	plan, err := BuildPlan(catalog, "test", testHosts()[:1], Overrides{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	state := newHostState(plan.Hosts[0])
	state.facts = Vars{"os_family": "debian", "arch": "x86_64"}
	tc := (Runner{Executor: executor}).taskContext(context.Background(), plan, state, NewPlayState())
	result := (Runner{Executor: executor}).executeTask(tc, task)
	if result.Status != batch.StatusOK || !strings.Contains(result.Output, "os=debian arch=x86_64") {
		t.Fatalf("facts 未进入参数渲染: %+v", result)
	}
}

func TestFailFastSkipsRemainingHosts(t *testing.T) {
	executor := &fakeExecutor{execFail: map[string]error{"/bin/false": &fakeExitError{code: 1}}}
	task := commandTask("boom", "/bin/false")
	play := Play{Name: "test", Hosts: deploy.TargetSelector{Hosts: []string{"h1", "h2"}}, Tasks: []Task{task}}
	catalog := &Catalog{ByName: map[string]Play{"test": play}}
	plan, err := BuildPlan(catalog, "test", testHosts(), Overrides{FailFast: true, Parallel: 1})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Failed != 1 || result.Summary.Skipped != 1 {
		t.Fatalf("fail-fast 摘要 = %+v", result.Summary)
	}
	for _, host := range result.Hosts {
		if len(host.Tasks) != 1 {
			t.Fatalf("主机 %s 应有 1 条任务记录: %+v", host.HostAlias, host.Tasks)
		}
	}
}

type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func (e *fakeExitError) ExitStatus() int {
	return e.code
}

var _ = batch.StatusOK

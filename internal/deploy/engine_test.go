package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"gopkg.in/yaml.v3"
)

func argsNode(values map[string]any) *yaml.Node {
	node := &yaml.Node{}
	data, err := yaml.Marshal(values)
	if err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal(data, node); err != nil {
		panic(err)
	}
	return node
}

type fakeExecutor struct {
	mu       sync.Mutex
	calls    []string
	stdins   []string
	execFail map[string]error
	execOut  map[string]string
	statInfo map[string]ops.RemoteFileInfo
	pushFunc func(options ops.TransferOptions) ops.Result
	dialTCP  func() error
}

func (f *fakeExecutor) Exec(_ context.Context, host config.Host, options ops.ExecOptions) ops.Result {
	command := options.Command
	canonical := command
	if argv, err := shellquote.Split(command); err == nil && len(argv) > 0 {
		canonical = strings.Join(argv, " ")
		if canonical != command {
			command += " [argv: " + canonical + "]"
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":exec:"+command)
	if options.Stdin != nil {
		data, _ := io.ReadAll(options.Stdin)
		f.stdins = append(f.stdins, string(data))
	} else {
		f.stdins = append(f.stdins, "")
	}
	f.mu.Unlock()
	out := "ok"
	if value, ok := f.execOut[options.Command]; ok {
		out = value
	} else if value, ok := f.execOut[canonical]; ok {
		out = value
	}
	if err, ok := f.execFail[options.Command]; ok {
		return ops.Result{Host: host, Stage: operation.StageExecute, Err: err, Output: out}
	} else if err, ok := f.execFail[canonical]; ok {
		return ops.Result{Host: host, Stage: operation.StageExecute, Err: err, Output: out}
	}
	return ops.Result{Host: host, OK: true, Output: out}
}

func (f *fakeExecutor) Push(_ context.Context, host config.Host, options ops.TransferOptions) ops.Result {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":push:"+options.Src+"->"+options.Dest)
	f.mu.Unlock()
	if f.pushFunc != nil {
		return f.pushFunc(options)
	}
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

func (f *fakeExecutor) DialTCP(_ context.Context, host config.Host, address string, _ time.Duration) (net.Conn, error) {
	f.mu.Lock()
	f.calls = append(f.calls, host.Alias+":dialtcp:"+address)
	f.mu.Unlock()
	if f.dialTCP != nil {
		if err := f.dialTCP(); err != nil {
			return nil, err
		}
		return &fakeNetConn{}, nil
	}
	return nil, errors.New("fake dialtcp")
}

type fakeNetConn struct{}

func (f *fakeNetConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (f *fakeNetConn) Write([]byte) (int, error)        { return 0, io.EOF }
func (f *fakeNetConn) Close() error                     { return nil }
func (f *fakeNetConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (f *fakeNetConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (f *fakeNetConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeNetConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeNetConn) SetWriteDeadline(time.Time) error { return nil }

func (f *fakeExecutor) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeExecutor) stdinLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stdins...)
}

func planFor(t *testing.T, tasks []Task, hosts []config.Host) *Plan {
	t.Helper()
	play := Play{Name: "test", Hosts: TargetSelector{Hosts: []string{"h1", "h2"}}, Tasks: tasks}
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

func commandArgvTask(name string, argv ...string) Task {
	return Task{
		Name: name, Module: "command",
		Args: argsNode(map[string]any{"argv": argv}),
	}
}

func TestLinearStrategyRunsTaskMajor(t *testing.T) {
	executor := &fakeExecutor{}
	plan := planFor(t, []Task{
		commandTask("first", "echo one"),
		commandTask("second", "echo two"),
	}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
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
	first.ChangedWhen = &Condition{RCIn: []int{0}}
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
	first.ChangedWhen = &Condition{RCIn: []int{99}}
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

func TestCommandExecutionReportsChangedAndCreatesSkipReportsOK(t *testing.T) {
	executor := &fakeExecutor{}
	executed := commandTask("execute", "echo changed")
	skipped := Task{
		Name: "skip-existing", Module: "command",
		Args: argsNode(map[string]any{"cmd": "echo skipped", "creates": "/tmp/already-there"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{executed, skipped}, testHosts()))
	if result.Summary.Changed != 2 {
		t.Fatalf("executed command should keep each host changed: %+v", result.Summary)
	}
	for _, host := range result.Hosts {
		if len(host.Tasks) != 2 || host.Tasks[0].Status != batch.StatusChanged || host.Tasks[1].Status != batch.StatusOK {
			t.Fatalf("command statuses = %+v", host.Tasks)
		}
		if host.Suggestion != "" {
			t.Fatalf("successful host should not carry failure suggestion: %+v", host)
		}
	}
	for _, call := range executor.callLog() {
		if strings.Contains(call, "echo skipped") {
			t.Fatalf("creates guard should skip command execution: %v", executor.callLog())
		}
	}
}

func TestChangedWhenFalseOverridesCommandDefault(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("probe", "echo probe")
	task.ChangedWhen = &Condition{RCIn: []int{99}}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.OK != 2 || result.Summary.Changed != 0 {
		t.Fatalf("changed_when=false should report OK: %+v", result.Summary)
	}
}

func TestIgnoreErrorsIsPreservedInTaskResult(t *testing.T) {
	executor := &fakeExecutor{execFail: map[string]error{"/bin/false": &fakeExitError{code: 7}}}
	task := commandTask("optional", "/bin/false")
	task.IgnoreErrors = true
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.OK != 2 || result.ExitCode() != 0 {
		t.Fatalf("ignored failure should continue successfully: %+v", result)
	}
	for _, host := range result.Hosts {
		if len(host.Tasks) != 1 || !host.Tasks[0].Ignored || host.Tasks[0].RC != 7 {
			t.Fatalf("ignored result metadata lost: %+v", host.Tasks)
		}
		if host.Suggestion != "" {
			t.Fatalf("ignored failure should not add a host failure suggestion: %+v", host)
		}
	}
}

func TestLocalModuleConfigFailureKeepsStageAndExitCode(t *testing.T) {
	executor := &fakeExecutor{}
	base := t.TempDir()
	task := Task{
		Name: "missing-local-source", Module: "copy", BaseDir: base,
		Args: argsNode(map[string]any{"src": "missing.txt", "dest": "/tmp/missing.txt"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.Failed != 2 || result.ExitCode() != 3 {
		t.Fatalf("local config failure should exit 3: %+v", result)
	}
	for _, host := range result.Hosts {
		if host.Stage != operation.StageConfig || len(host.Tasks) != 1 || host.Tasks[0].Stage != operation.StageConfig {
			t.Fatalf("config stage should survive module/host aggregation: %+v", host)
		}
	}
	if len(executor.callLog()) != 0 {
		t.Fatalf("missing local source should fail before remote access: %v", executor.callLog())
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
	if result.Summary.Failed != 0 || result.Summary.Changed != 2 {
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
	task := commandArgvTask("render", "echo", "{{ item }}")
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

func TestLoopWhenFiltersItems(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandArgvTask("render", "echo", "{{ item }}")
	task.Loop = []string{"a", "skip", "b"}
	task.When = "item != 'skip'"
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	calls := executor.callLog()
	a, b, skip := 0, 0, 0
	for _, call := range calls {
		switch {
		case strings.Contains(call, "echo a"):
			a++
		case strings.Contains(call, "echo b"):
			b++
		case strings.Contains(call, "echo skip"):
			skip++
		}
	}
	if a != 2 || b != 2 || skip != 0 {
		t.Fatalf("a=%d b=%d skip=%d; calls=%v", a, b, skip, calls)
	}
}

func TestLoopWhenAllSkippedMarksHostSkipped(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandArgvTask("render", "echo", "{{ item }}")
	task.Loop = []string{"a", "b"}
	task.When = "item == 'never'"
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Skipped != 2 {
		t.Fatalf("全部 item 跳过应标记主机 skipped: %+v", result.Summary)
	}
	if len(executor.callLog()) != 0 {
		t.Fatalf("不应执行任何命令: %v", executor.callLog())
	}
}

func TestLoopWhenRegisterOnlyMatchedItems(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandArgvTask("render", "echo", "{{ item }}")
	task.Loop = []string{"a", "skip", "b"}
	task.When = "item != 'skip'"
	task.Register = "results"
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	for _, host := range result.Hosts {
		for _, taskResult := range host.Tasks {
			register, ok := taskResult.Register.([]any)
			if !ok {
				t.Fatalf("register 应为列表: %+v", taskResult.Register)
			}
			if len(register) != 2 {
				t.Fatalf("被过滤的 item 不应进入 register: %+v", register)
			}
			for _, item := range register {
				entry, ok := item.(map[string]any)
				if !ok || entry["status"] != string(batch.StatusChanged) || entry["output"] != "ok" || entry["changed"] != true {
					t.Fatalf("loop register 应保留每次执行结果: %+v", register)
				}
			}
		}
	}
}

func TestCopyModeConvergesWhenContentAlreadyMatches(t *testing.T) {
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, Mode: 0o644},
		},
		pushFunc: func(options ops.TransferOptions) ops.Result {
			return ops.Result{Host: config.Host{}, OK: true, Changed: false, Destination: options.Dest}
		},
	}
	task := Task{
		Name: "ensure-mode", Module: "copy",
		Args: argsNode(map[string]any{"content": "same", "dest": "/tmp/app.conf", "mode": "0600"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("内容一致但权限漂移应触发 chmod: %+v", result.Summary)
	}
	calls := executor.callLog()
	chmod := 0
	for _, call := range calls {
		if strings.Contains(call, "chmod 0600") {
			chmod++
		}
	}
	if chmod != 2 {
		t.Fatalf("chmod 次数 = %d, want 2; calls=%v", chmod, calls)
	}
}

func TestCopyModeSkipsRedundantChmod(t *testing.T) {
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, Mode: 0o600},
		},
		pushFunc: func(options ops.TransferOptions) ops.Result {
			return ops.Result{Host: config.Host{}, OK: true, Changed: true, Destination: options.Dest}
		},
	}
	task := Task{
		Name: "upload", Module: "copy",
		Args: argsNode(map[string]any{"content": "new", "dest": "/tmp/app.conf", "mode": "0600"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	(Runner{Executor: executor}).Run(context.Background(), plan)
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chmod") {
			t.Fatalf("内容变化且权限已匹配时不应 chmod: %v", executor.callLog())
		}
	}
}

func TestCopyModeRejectsRemoteSymlink(t *testing.T) {
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, IsLink: true, Mode: os.ModeSymlink | 0777},
		},
	}
	task := Task{
		Name: "copy", Module: "copy",
		Args: argsNode(map[string]any{"content": "x", "dest": "/tmp/app.conf", "mode": "0600"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.Failed != 2 {
		t.Fatalf("remote symlinks must be rejected before chmod: %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chmod") {
			t.Fatalf("chmod must not follow a remote symlink: %v", executor.callLog())
		}
	}
}

func TestFileStateFileRejectsRemoteSymlink(t *testing.T) {
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, IsLink: true, Mode: os.ModeSymlink | 0777},
		},
	}
	task := Task{
		Name: "file", Module: "file",
		Args: argsNode(map[string]any{"path": "/tmp/app.conf", "state": "file", "owner": "root"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.Failed != 2 {
		t.Fatalf("remote symlinks must be rejected before metadata changes: %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chmod") || strings.Contains(call, "chown") {
			t.Fatalf("metadata commands must not follow a remote symlink: %v", executor.callLog())
		}
	}
}

func TestFileOwnershipIsIdempotentWhenNameOrIDMatches(t *testing.T) {
	probe := ownershipProbeCommand("/tmp/app.conf")
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, Mode: 0o640},
		},
		execOut: map[string]string{probe: "root:app:0:1000\n"},
	}
	task := Task{
		Name: "file", Module: "file",
		Args: argsNode(map[string]any{"path": "/tmp/app.conf", "state": "file", "owner": "0", "group": "app"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.OK != 2 || result.Summary.Changed != 0 {
		t.Fatalf("matching ownership should be idempotent: %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chown") {
			t.Fatalf("matching ownership should not run chown: %v", executor.callLog())
		}
	}
}

func TestFileOwnershipChangesOnlyWhenDifferent(t *testing.T) {
	probe := ownershipProbeCommand("/tmp/app.conf")
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, Mode: 0o640},
		},
		execOut: map[string]string{probe: "deploy:deploy:1000:1000\n"},
	}
	task := Task{
		Name: "file", Module: "file",
		Args: argsNode(map[string]any{"path": "/tmp/app.conf", "state": "file", "owner": "root", "group": "root"}),
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.Changed != 2 {
		t.Fatalf("different ownership should change: %+v", result.Summary)
	}
	chowns := 0
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chown") {
			chowns++
		}
	}
	if chowns != 2 {
		t.Fatalf("chown calls = %d, want 2: %v", chowns, executor.callLog())
	}
}

func TestFileAbsentRejectsBroadDestructivePaths(t *testing.T) {
	module := &fileModule{}
	for _, target := range []string{"/", ".", "..", "~", "~/"} {
		if _, err := module.DecodeArgs(argsNode(map[string]any{"path": target, "state": "absent"})); err == nil {
			t.Fatalf("file absent should reject broad target %q", target)
		}
	}
}

func TestCopyModeCheckHasNoSideEffects(t *testing.T) {
	executor := &fakeExecutor{
		statInfo: map[string]ops.RemoteFileInfo{
			"/tmp/app.conf": {Exists: true, Mode: 0o644},
		},
		pushFunc: func(options ops.TransferOptions) ops.Result {
			return ops.Result{Host: config.Host{}, OK: true, Changed: true, WouldChange: true, Destination: options.Dest}
		},
	}
	task := Task{
		Name: "ensure-mode", Module: "copy",
		Args: argsNode(map[string]any{"content": "x", "dest": "/tmp/app.conf", "mode": "0600"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.WouldChange != 2 {
		t.Fatalf("check 模式应报告 would-change: %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if strings.Contains(call, "chmod") {
			t.Fatalf("check 模式不应执行 chmod: %v", executor.callLog())
		}
	}
}

func TestCopyRejectsInvalidMode(t *testing.T) {
	module := &copyModule{}
	if _, err := module.DecodeArgs(argsNode(map[string]any{"content": "x", "dest": "/tmp/a", "mode": "abc"})); err == nil {
		t.Fatal("copy mode 非八进制应当报错")
	}
	if _, err := module.DecodeArgs(argsNode(map[string]any{"content": "x", "dest": "/tmp/a", "mode": "0755"})); err != nil {
		t.Fatalf("copy mode 0755 应合法: %v", err)
	}
}

func TestCopyAcceptsExplicitEmptyContent(t *testing.T) {
	module := &copyModule{}
	raw, err := module.DecodeArgs(argsNode(map[string]any{"content": "", "dest": "/tmp/empty"}))
	if err != nil {
		t.Fatalf("explicit empty content should be valid: %v", err)
	}
	args := raw.(*copyArgs)
	if args.Content == nil || *args.Content != "" || args.Src != "" {
		t.Fatalf("decoded empty content = %+v", args)
	}
}

func TestBlockRetainsChildOutput(t *testing.T) {
	executor := &fakeExecutor{}
	blockTask := Task{
		Name: "publish",
		Block: []Task{
			commandTask("first", "echo alpha"),
			commandTask("second", "echo beta"),
		},
	}
	plan := planFor(t, []Task{blockTask}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	for _, host := range result.Hosts {
		if len(host.Tasks) != 1 || host.Tasks[0].Output != "okok" {
			t.Fatalf("block 子任务输出未聚合: %+v", host.Tasks)
		}
	}
}

func TestBlockAggregatesChangedStatusFromAlways(t *testing.T) {
	executor := &fakeExecutor{
		pushFunc: func(options ops.TransferOptions) ops.Result {
			return ops.Result{OK: true, Changed: true, WouldChange: options.Check, Destination: options.Dest}
		},
	}
	blockTask := Task{
		Name:  "publish",
		Block: []Task{{Name: "main", Module: "debug", Args: argsNode(map[string]any{"msg": "main"})}},
		Always: []Task{{
			Name: "always-write", Module: "copy",
			Args: argsNode(map[string]any{"content": "written", "dest": "/tmp/always.txt"}),
		}},
	}
	result := (Runner{Executor: executor}).Run(context.Background(), planFor(t, []Task{blockTask}, testHosts()))
	if result.Summary.Changed != 2 || result.Summary.OK != 0 {
		t.Fatalf("always change should propagate to block: %+v", result.Summary)
	}
}

func TestBlockAggregatesWouldChangeStatusFromAlwaysInCheckMode(t *testing.T) {
	executor := &fakeExecutor{
		pushFunc: func(options ops.TransferOptions) ops.Result {
			return ops.Result{OK: true, Changed: true, WouldChange: options.Check, Destination: options.Dest}
		},
	}
	blockTask := Task{
		Name:  "publish",
		Block: []Task{{Name: "main", Module: "debug", Args: argsNode(map[string]any{"msg": "main"})}},
		Always: []Task{{
			Name: "always-write", Module: "copy",
			Args: argsNode(map[string]any{"content": "written", "dest": "/tmp/always.txt"}),
		}},
	}
	plan := planFor(t, []Task{blockTask}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.WouldChange != 2 || result.Summary.Changed != 0 {
		t.Fatalf("always check change should propagate as would-change: %+v", result.Summary)
	}
}

func TestSleepWaitsAndIsSkippedInCheck(t *testing.T) {
	executor := &fakeExecutor{}
	task := Task{
		Name: "wait-a-bit", Module: "sleep",
		Args: argsNode(map[string]any{"duration": "20ms"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 {
		t.Fatalf("sleep 应成功: %+v", result.Summary)
	}
	if len(executor.callLog()) != 0 {
		t.Fatalf("sleep 不应调用执行器: %v", executor.callLog())
	}
	plan.Check = true
	result = (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Skipped != 2 {
		t.Fatalf("check 模式 sleep 应跳过: %+v", result.Summary)
	}
}

func TestSleepIgnoresShortTaskTimeout(t *testing.T) {
	executor := &fakeExecutor{}
	task := Task{
		Name: "wait-a-bit", Module: "sleep",
		Args: argsNode(map[string]any{"duration": "1500ms"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	plan.Timeout = config.Duration{Duration: time.Second}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 {
		t.Fatalf("sleep 不应被短任务超时截断: %+v", result.Summary)
	}
}

func TestSleepRejectsInvalidArgs(t *testing.T) {
	module := &sleepModule{}
	for _, args := range []map[string]any{
		{},
		{"seconds": 5, "duration": "5s"},
		{"seconds": 0},
		{"duration": "0s"},
		{"seconds": 86401},
		{"duration": "25h"},
	} {
		if _, err := module.DecodeArgs(argsNode(args)); err == nil {
			t.Fatalf("sleep 参数 %v 应当报错", args)
		}
	}
	if _, err := module.DecodeArgs(argsNode(map[string]any{"seconds": 3})); err != nil {
		t.Fatalf("seconds 参数应合法: %v", err)
	}
	if _, err := module.DecodeArgs(argsNode(map[string]any{"duration": "5s"})); err != nil {
		t.Fatalf("duration 参数应合法: %v", err)
	}
}

func TestFreeStrategyEmitsPerTaskEvents(t *testing.T) {
	executor := &fakeExecutor{}
	plan := planFor(t, []Task{
		commandTask("first", "echo one"),
		commandTask("second", "echo two"),
	}, testHosts())
	plan.Strategy = StrategyFree
	var events []PlayEvent
	result := (Runner{Executor: executor, Event: func(event PlayEvent) {
		events = append(events, event)
	}}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	taskEvents := 0
	for _, event := range events {
		if event.Type != EventTaskDone {
			continue
		}
		taskEvents++
		if event.Task == "" || event.TaskIndex < 0 || event.Host == "" {
			t.Fatalf("free 事件缺少任务信息: %+v", event)
		}
	}
	if taskEvents != 4 {
		t.Fatalf("free 策略应按任务发 4 条事件，实际 %d: %+v", taskEvents, events)
	}
}

func TestGatherFactsRunsThroughEngine(t *testing.T) {
	t.Setenv("SSHMD_HOME", t.TempDir())
	executor := &fakeExecutor{}
	task := Task{
		Name: "debug-facts", Module: "debug",
		Args: argsNode(map[string]any{"msg": "host={{ hostname }}"}),
	}
	play := Play{Name: "test", Hosts: TargetSelector{Hosts: []string{"h1", "h2"}}, GatherFacts: true, Tasks: []Task{task}}
	catalog := &Catalog{ByName: map[string]Play{"test": play}}
	plan, err := BuildPlan(catalog, "test", testHosts(), Overrides{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 {
		t.Fatalf("gather_facts 后任务应成功: %+v", result.Summary)
	}
	for _, host := range result.Hosts {
		for _, taskResult := range host.Tasks {
			if !strings.Contains(taskResult.Output, "host=") {
				t.Fatalf("facts 未进入任务环境: %+v", taskResult)
			}
		}
	}
}

func TestCheckModeSkippedDoesNotFailExitCode(t *testing.T) {
	if code := (RunResult{Check: true, Summary: batch.Summary{Skipped: 2}}).ExitCode(); code != 0 {
		t.Fatalf("check 模式下 skipped 不应失败，退出码 = %d", code)
	}
	if code := (RunResult{Check: false, Summary: batch.Summary{Skipped: 2}}).ExitCode(); code != 1 {
		t.Fatalf("非 check 模式 skipped 应失败，退出码 = %d", code)
	}
	if code := (RunResult{Check: true, Summary: batch.Summary{Failed: 1}}).ExitCode(); code != 1 {
		t.Fatalf("check 模式 failed 仍应失败，退出码 = %d", code)
	}
}

func TestCheckModeCommandRunsExitZero(t *testing.T) {
	executor := &fakeExecutor{}
	plan := planFor(t, []Task{commandTask("probe", "hostname")}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Skipped != 2 {
		t.Fatalf("check 模式 command 应跳过: %+v", result.Summary)
	}
	if code := result.ExitCode(); code != 0 {
		t.Fatalf("check 模式退出码 = %d, want 0", code)
	}
	for _, host := range result.Hosts {
		for _, taskResult := range host.Tasks {
			if !strings.Contains(taskResult.Reason, "check 模式跳过") {
				t.Fatalf("check 跳过原因应明确: %+v", taskResult)
			}
		}
	}
}

func TestCheckSafeCommandRunsWithoutReportingChange(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("probe", "hostname")
	task.CheckSafe = true
	plan := planFor(t, []Task{task}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 || result.Summary.Changed != 0 || result.Summary.WouldChange != 0 {
		t.Fatalf("check_safe probe should stay read-only: %+v", result.Summary)
	}
	if len(executor.callLog()) != 2 {
		t.Fatalf("check_safe command should execute once per host: %v", executor.callLog())
	}
}

func TestCheckSafeChangedWhenCanExplicitlyReportWouldChange(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("probe", "hostname")
	task.CheckSafe = true
	task.ChangedWhen = &Condition{RCIn: []int{0}}
	plan := planFor(t, []Task{task}, testHosts())
	plan.Check = true
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.WouldChange != 2 || result.Summary.Changed != 0 {
		t.Fatalf("explicit changed_when should report would-change in check mode: %+v", result.Summary)
	}
}

func TestConfirmGatesEachSerialBatch(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Batch.Serial = 1
	prompts := 0
	result := (Runner{Executor: executor, Confirm: func(message string) error {
		if message != "确认重启?" {
			t.Fatalf("confirm 消息 = %q", message)
		}
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	if prompts != 2 {
		t.Fatalf("serial=1 的两台主机应各确认一次，实际 %d 次", prompts)
	}
}

func TestConfirmAbortsRun(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Batch.Serial = 1
	rejected := fmt.Errorf("用户拒绝")
	result := (Runner{Executor: executor, Confirm: func(string) error { return rejected }}).Run(context.Background(), plan)
	if result.StopReason == "" || !strings.Contains(result.StopReason, "用户拒绝") {
		t.Fatalf("StopReason = %q", result.StopReason)
	}
	if result.Summary.Skipped != 2 {
		t.Fatalf("拒绝后主机应全部跳过: %+v", result.Summary)
	}
}

func TestConfirmSkippedInCheck(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Check = true
	prompts := 0
	(Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if prompts != 0 {
		t.Fatalf("check 模式不应触发 confirm，实际 %d 次", prompts)
	}
}

func TestFreeStrategyConfirmsEachMessageOnce(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	result := (Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	if prompts != 1 {
		t.Fatalf("free 策略应只确认一次，实际 %d 次", prompts)
	}
}

func TestFreeStrategyConfirmationRejectionStopsEveryHost(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	result := (Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return errors.New("用户拒绝")
	}}).Run(context.Background(), plan)
	if prompts != 1 || result.Summary.Failed != 2 || result.ExitCode() != 1 {
		t.Fatalf("rejected confirmation should fail every host once: prompts=%d result=%+v", prompts, result)
	}
	if len(executor.callLog()) != 0 {
		t.Fatalf("no host may execute after shared confirmation rejection: %v", executor.callLog())
	}
	for _, host := range result.Hosts {
		if host.Stage != operation.StageConfirm || host.Suggestion != operation.Suggestion(operation.StageConfirm) {
			t.Fatalf("confirmation failure metadata = %+v", host)
		}
	}
}

func TestFreeStrategyConfirmSkipsTasksThatNeverRun(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("restart", "echo restart")
	task.Confirm = "确认重启?"
	task.When = "1 == 2"
	plan := planFor(t, []Task{task}, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	result := (Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if result.Summary.Skipped != 2 {
		t.Fatalf("when 不满足的主机应跳过: %+v", result.Summary)
	}
	if prompts != 0 {
		t.Fatalf("不会执行的任务不应触发 confirm，实际 %d 次", prompts)
	}
}

func TestFreeStrategyConfirmDeduplicatesMessages(t *testing.T) {
	executor := &fakeExecutor{}
	first := commandTask("a", "echo a")
	first.Confirm = "确认 A?"
	second := commandTask("b", "echo b")
	second.Confirm = "确认 B?"
	plan := planFor(t, []Task{first, second}, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	(Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if prompts != 2 {
		t.Fatalf("两个不同 confirm 消息应各确认一次，实际 %d 次", prompts)
	}
}

func TestFreeStrategyDoesNotDeduplicateDifferentTasksWithSameMessage(t *testing.T) {
	executor := &fakeExecutor{}
	first := commandTask("a", "echo a")
	first.Confirm = "确认继续?"
	second := commandTask("b", "echo b")
	second.Confirm = "确认继续?"
	plan := planFor(t, []Task{first, second}, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	(Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if prompts != 2 {
		t.Fatalf("相同文案的两个不同任务应分别确认，实际 %d 次", prompts)
	}
}

func TestPauseSameMessagePromptsOncePerTask(t *testing.T) {
	executor := &fakeExecutor{}
	tasks := []Task{
		{Name: "gate-a", Module: "pause", Args: argsNode(map[string]any{"message": "gate"})},
		{Name: "gate-b", Module: "pause", Args: argsNode(map[string]any{"message": "gate"})},
	}
	plan := planFor(t, tasks, testHosts())
	plan.Strategy = StrategyFree
	prompts := 0
	result := (Runner{Executor: executor, Confirm: func(string) error {
		prompts++
		return nil
	}}).Run(context.Background(), plan)
	if result.Summary.Failed != 0 || prompts != 2 {
		t.Fatalf("pause prompts=%d summary=%+v", prompts, result.Summary)
	}
}

func TestPauseRejectionUsesConfirmationStage(t *testing.T) {
	executor := &fakeExecutor{}
	task := Task{Name: "gate", Module: "pause", Args: argsNode(map[string]any{"message": "gate"})}
	result := (Runner{Executor: executor, Confirm: func(string) error {
		return errors.New("用户拒绝")
	}}).Run(context.Background(), planFor(t, []Task{task}, testHosts()))
	if result.Summary.Failed != 2 || result.ExitCode() != 1 {
		t.Fatalf("pause rejection should be an operational failure: %+v", result)
	}
	for _, host := range result.Hosts {
		if host.Stage != operation.StageConfirm {
			t.Fatalf("pause rejection stage = %+v", host)
		}
	}
}

func TestPlayStateSerializesDifferentConfirmPrompts(t *testing.T) {
	state := NewPlayState()
	entered := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	confirm := func(message string) error {
		entered <- message
		<-release
		return nil
	}

	go func() { done <- state.ConfirmOnce("task-a", "确认 A?", confirm) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first confirmation did not start")
	}
	go func() { done <- state.ConfirmOnce("task-b", "确认 B?", confirm) }()
	select {
	case message := <-entered:
		t.Fatalf("confirm callbacks overlapped: %s", message)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second confirmation did not run after the first completed")
	}
}

func TestServiceUnmasksMaskedService(t *testing.T) {
	command := "systemctl is-enabled -- 'svc'"
	executor := &fakeExecutor{
		execFail: map[string]error{command: &fakeExitError{code: 1}},
		execOut:  map[string]string{command: "masked\n"},
	}
	task := Task{
		Name: "ensure", Module: "service",
		Args: argsNode(map[string]any{"name": "svc", "state": "enabled"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	calls := strings.Join(executor.callLog(), "\n")
	if !strings.Contains(calls, "systemctl unmask -- 'svc'") || !strings.Contains(calls, "systemctl enable -- 'svc'") {
		t.Fatalf("masked 服务应先 unmask 再 enable: %v", executor.callLog())
	}
}

func TestWaitForTargetConnectFrom(t *testing.T) {
	executor := &fakeExecutor{dialTCP: func() error { return nil }}
	task := Task{
		Name: "wait", Module: "wait_for",
		Args: argsNode(map[string]any{"port": 8080, "connect_from": "target"}),
	}
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	if result.Summary.OK != 2 {
		t.Fatalf("target 模式连接成功应直接 ok: %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if !strings.Contains(call, ":dialtcp:") {
			t.Fatalf("target 模式应通过 SSH 拨号: %v", executor.callLog())
		}
	}
}

func TestWaitForRejectsBadConnectFrom(t *testing.T) {
	module := &waitForModule{}
	if _, err := module.DecodeArgs(argsNode(map[string]any{"port": 8080, "connect_from": "bogus"})); err == nil {
		t.Fatal("connect_from 非法值应当报错")
	}
}

func TestDebugVarLooksUpRegisters(t *testing.T) {
	executor := &fakeExecutor{}
	first := commandTask("upload", "echo upload")
	first.Register = "upload"
	first.ChangedWhen = &Condition{RCIn: []int{0}}
	second := Task{
		Name: "inspect", Module: "debug",
		Args: argsNode(map[string]any{"var": "upload.changed"}),
	}
	plan := planFor(t, []Task{first, second}, testHosts())
	result := (Runner{Executor: executor}).Run(context.Background(), plan)
	outputs := ""
	for _, host := range result.Hosts {
		for _, taskResult := range host.Tasks {
			outputs += taskResult.Output
		}
	}
	if !strings.Contains(outputs, "true") {
		t.Fatalf("debug var 应能读取 register: %q", outputs)
	}
}

func TestCommandModuleQuotesShellGrammarAsLiteralArguments(t *testing.T) {
	command := &commandModule{name: "command"}
	decoded, err := command.DecodeArgs(argsNode(map[string]any{"cmd": `echo "(id)" '$HOME;touch /tmp/pwn'`}))
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(*commandArgs).Cmd
	if got != `'echo' '(id)' '$HOME;touch /tmp/pwn'` {
		t.Fatalf("command should quote every argv value: %q", got)
	}
	shell := &commandModule{name: "shell"}
	if _, err := shell.DecodeArgs(argsNode(map[string]any{"cmd": "ls | wc -l"})); err != nil {
		t.Fatalf("shell 模块应允许管道: %v", err)
	}
}

func TestRunRemoteRejectsInvalidEnvironmentName(t *testing.T) {
	result := runRemote(TaskContext{Env: map[string]string{"SAFE;id": "x"}}, "true")
	if result.Err == nil || result.Stage != operation.StageConfig {
		t.Fatalf("invalid env name result = %+v", result)
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
	play := Play{Name: "test", Hosts: TargetSelector{Hosts: []string{"h1"}}, Tasks: []Task{task}}
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
	play := Play{Name: "test", Hosts: TargetSelector{Hosts: []string{"h1", "h2"}}, Tasks: []Task{task}}
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

func TestBecomeWithPasswordUsesSudoS(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("become", "whoami")
	task.Become = true
	plan := planFor(t, []Task{task}, testHosts())
	result := (Runner{
		Executor: executor,
		BecomePassword: func(host config.Host) (string, bool) {
			return "secret", true
		},
	}).Run(context.Background(), plan)
	if result.Summary.Changed != 2 {
		t.Fatalf("summary = %+v", result.Summary)
	}
	for _, call := range executor.callLog() {
		if !strings.Contains(call, "sudo -S -p ''") {
			t.Fatalf("become 密码模式应使用 sudo -S: %s", call)
		}
		if strings.Contains(call, "sudo -n") {
			t.Fatalf("become 密码模式不应使用 sudo -n: %s", call)
		}
	}
	for _, stdin := range executor.stdinLog() {
		if stdin != "secret\n" {
			t.Fatalf("sudo 密码应经 stdin 传入: %q", stdin)
		}
	}
}

func TestBecomeWithoutPasswordUsesSudoN(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("become", "whoami")
	task.Become = true
	plan := planFor(t, []Task{task}, testHosts())
	(Runner{Executor: executor}).Run(context.Background(), plan)
	for _, call := range executor.callLog() {
		if !strings.Contains(call, "sudo -n") {
			t.Fatalf("无密码时 become 应使用 sudo -n: %s", call)
		}
	}
	for _, stdin := range executor.stdinLog() {
		if stdin != "" {
			t.Fatalf("无密码时不应提供 stdin: %q", stdin)
		}
	}
}

func TestBecomePasswordFromRunnerResolver(t *testing.T) {
	executor := &fakeExecutor{}
	task := commandTask("become", "whoami")
	task.Become = true
	plan := planFor(t, []Task{task}, testHosts())
	runner := Runner{
		Executor: executor,
		BecomePassword: func(host config.Host) (string, bool) {
			return "vault-pwd", true
		},
	}
	runner.Run(context.Background(), plan)
	for _, stdin := range executor.stdinLog() {
		if stdin != "vault-pwd\n" {
			t.Fatalf("resolver 密码应经 stdin 传入: %q", stdin)
		}
	}
}

func TestTaskRejectsBecomePasswordInPlaybook(t *testing.T) {
	var task Task
	err := yaml.Unmarshal([]byte(`name: privileged
become: true
become_password: plaintext-secret
command:
  cmd: whoami
`), &task)
	if err == nil || !strings.Contains(err.Error(), "禁止保存 become_password") {
		t.Fatalf("become_password should be rejected, error = %v", err)
	}
}

func TestCommandTemplateRequiresStructuredArgv(t *testing.T) {
	unsafe := commandTask("unsafe", "rm -- {{ target }}")
	if err := validateTask(unsafe, Vars{"target": "safe /etc/critical"}); err == nil || !strings.Contains(err.Error(), "argv") {
		t.Fatalf("templated cmd should require argv: %v", err)
	}

	safe := Task{
		Name:   "safe",
		Module: "command",
		Args: argsNode(map[string]any{
			"argv": []string{"rm", "--", "{{ target }}"},
		}),
	}
	vars := Vars{"target": "safe /etc/critical"}
	if err := validateTask(safe, vars); err != nil {
		t.Fatalf("structured argv should validate: %v", err)
	}
	rendered, err := renderArgs(safe.Args, vars)
	if err != nil {
		t.Fatal(err)
	}
	module, _ := Lookup("command")
	decoded, err := module.DecodeArgs(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.(*commandArgs).Cmd; got != "'rm' '--' 'safe /etc/critical'" {
		t.Fatalf("rendered command = %q", got)
	}
}

var _ = batch.StatusOK

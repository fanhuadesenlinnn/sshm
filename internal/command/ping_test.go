package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
)

func TestPingSingleHostUnreachableExitCode(t *testing.T) {
	t.Setenv("SSHMD_HOME", t.TempDir())
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshmd.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "dead", "root", "127.0.0.1", 1
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	err := app.cmdPing([]string{"dead"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("应返回 ExitError: %v", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("单主机不可达退出码 = %d, want 2", exitErr.Code)
	}
}

func TestRunPingBatchActuallyRunsHostsInParallel(t *testing.T) {
	hosts := []config.Host{{Alias: "one"}, {Alias: "two"}, {Alias: "three"}}
	started := make(chan string, len(hosts))
	release := make(chan struct{})
	done := make(chan batchPingResult, 1)
	go func() {
		result, err := runPingBatch(context.Background(), hosts, 3, time.Second, nil,
			func(_ context.Context, host config.Host, _ *secret.FileStore) (string, error) {
				started <- host.Alias
				<-release
				return "ok\n", nil
			}, nil)
		done <- batchPingResult{ok: result.Summary.OK, err: err}
	}()
	for range hosts {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("all ping workers should start before any worker is released")
		}
	}
	close(release)
	result := <-done
	if result.err != nil || result.ok != len(hosts) {
		t.Fatalf("parallel ping result: ok=%d err=%v", result.ok, result.err)
	}
}

func TestRunPingBatchPreservesFailureStage(t *testing.T) {
	host := config.Host{Alias: "one"}
	result, err := runPingBatch(context.Background(), []config.Host{host}, 1, time.Second, nil,
		func(context.Context, config.Host, *secret.FileStore) (string, error) {
			return "", operation.Wrap(operation.StageNetwork, errors.New("down"))
		}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Unreachable != 1 {
		t.Fatalf("network failure summary = %+v", result.Summary)
	}
}

type batchPingResult struct {
	ok  int
	err error
}

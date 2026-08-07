package deployv3

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

func TestGatherFacts(t *testing.T) {
	host := config.Host{Alias: "web01", Host: "10.0.0.1", User: "root", Port: 22}
	factsExecutor := &factsFakeExecutor{}
	cacheDir := filepath.Join(t.TempDir(), "facts")
	facts, err := gatherFacts(context.Background(), host, factsExecutor, 1000000000, cacheDir)
	if err != nil {
		t.Fatalf("gatherFacts: %v", err)
	}
	if facts["hostname"] != "web01-host" || facts["system"] != "Linux" || facts["arch"] != "x86_64" {
		t.Fatalf("facts = %+v", facts)
	}
	if facts["os_family"] != "debian" {
		t.Fatalf("os_family = %v, want debian", facts["os_family"])
	}
}

type factsFakeExecutor struct{}

func (f *factsFakeExecutor) Exec(_ context.Context, host config.Host, options ops.ExecOptions) ops.Result {
	output := "web01-host\nLinux\nx86_64\nID=ubuntu\n"
	return ops.Result{Host: host, OK: true, Output: output}
}

func (f *factsFakeExecutor) Push(context.Context, config.Host, ops.TransferOptions) ops.Result {
	return ops.Result{OK: true}
}

func (f *factsFakeExecutor) Pull(context.Context, config.Host, ops.TransferOptions) ops.Result {
	return ops.Result{OK: true}
}

func (f *factsFakeExecutor) Stat(context.Context, config.Host, string, time.Duration) (ops.RemoteFileInfo, error) {
	return ops.RemoteFileInfo{}, nil
}

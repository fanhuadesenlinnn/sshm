package operation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestWriteLogCreatesPerHostFiles(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	path, err := WriteLog("exec-batch", "uname", []Result{{
		Host:   config.Host{Alias: "prod", Host: "10.0.0.1", User: "root", Port: 22},
		Output: "Linux\n", Err: errors.New("failed"), Stage: StageExecute,
		RetryCommand: "sshm exec --yes prod uname",
	}})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := os.ReadFile(filepath.Join(path, "summary.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(summary), "prod") {
		t.Fatalf("summary missing host: %s", summary)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 2 {
		t.Fatalf("log entries = %v, %v", entries, err)
	}
}

func TestWriteLogPreservesExplicitSkippedStatus(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	path, err := WriteLog("exec-batch", "uname", []Result{{
		Host: config.Host{Alias: "prod", Host: "10.0.0.1"}, Status: "skipped", Output: "skipped: fail-fast\n",
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "prod-10.0.0.1.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: skipped") || !strings.Contains(string(data), "fail-fast") {
		t.Fatalf("log = %s", data)
	}
}

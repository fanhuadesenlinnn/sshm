package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func TestHostLogFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"web01-10.0.0.1.log",
		"db01-10.0.0.2.log",
		"summary.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	matches := hostLogFiles(dir, "web01")
	if len(matches) != 1 || !strings.Contains(matches[0], "web01-10.0.0.1.log") {
		t.Fatalf("matches = %v", matches)
	}
	if len(hostLogFiles(dir, "missing")) != 0 {
		t.Fatal("不存在的主机不应匹配")
	}
}

func TestLogActionFilterMatchesSuffixedRunDirectories(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSHMD_HOME", dir)
	for _, name := range []string{
		"20260808-000000000000000000-exec",
		"20260808-000000000000000001-exec-batch",
		"20260808-000000000000000002-push-batch",
		"20260808-000000000000000003-deploy-publish",
		"20260808-000000000000000004-config",
	} {
		if err := os.MkdirAll(filepath.Join(config.LogsDir(), name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	collect := func(action string) []string {
		var out []string
		for _, entry := range logDirectoriesMatching(action) {
			out = append(out, filepath.Base(entry))
		}
		return out
	}
	if got := collect("exec"); len(got) != 2 {
		t.Fatalf("--action exec 应匹配 -exec 与 -exec-batch: %v", got)
	}
	if got := collect("push"); len(got) != 1 {
		t.Fatalf("--action push 应匹配 -push-batch: %v", got)
	}
	if got := collect("deploy"); len(got) != 1 {
		t.Fatalf("--action deploy 应匹配 -deploy-publish: %v", got)
	}
	if got := collect("nope"); len(got) != 0 {
		t.Fatalf("未知 action 不应匹配: %v", got)
	}
}

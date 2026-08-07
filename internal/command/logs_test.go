package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProgressNoopsWhenStderrIsNotTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = file
	defer func() {
		os.Stderr = originalStderr
		file.Close()
	}()

	RefreshLine("  [%d/%d] 执行中...", 1, 2)
	EndProgress()
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("progress should be silent for non-terminal stderr, got %q", data)
	}
}

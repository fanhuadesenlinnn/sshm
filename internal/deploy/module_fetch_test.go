package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullDestinationRejectsRelativeEscape(t *testing.T) {
	base := t.TempDir()
	if _, err := pullDestination(base, base, "web01", "../outside", "/var/log/app.log", false); err == nil {
		t.Fatal("relative fetch destination should not escape playbook directory")
	}
}

func TestPullDestinationRejectsSymlinkBelowPlaybook(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "downloads")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := pullDestination(base, base, "web01", "downloads", "/var/log/app.log", false)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("pullDestination() error = %v, want symlink rejection", err)
	}
}

func TestPullDestinationRejectsSymlinkInHostSubdirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "downloads")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "web01")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := pullDestination(base, base, "web01", "downloads", "/var/log/app.log", false)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("host subdirectory symlink error = %v", err)
	}
}

func TestPullDestinationRejectsAbsoluteControllerPath(t *testing.T) {
	base := t.TempDir()
	if _, err := pullDestination(base, base, "web01", filepath.Join(t.TempDir(), "outside"), "/var/log/app.log", true); err == nil {
		t.Fatal("fetch must not accept an absolute controller destination")
	}
}

package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiPullDestinationUsesFetchLayout(t *testing.T) {
	root := t.TempDir()
	got, err := multiPullDestination(root, "web01", "/etc/nginx/nginx.conf", false)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "web01", "etc", "nginx", "nginx.conf")
	if got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestConfinedJoinRejectsSymlinkBelowDownloadRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "web01")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := multiPullDestination(root, "web01", "/etc/hosts", false); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("multiPullDestination() error = %v, want symlink rejection", err)
	}
}

func TestMultiPullDestinationRejectsTraversalAndUnsafeAlias(t *testing.T) {
	for _, remote := range []string{"../secret", "/etc/../secret", ".", "/"} {
		if _, err := multiPullDestination(t.TempDir(), "web01", remote, false); err == nil {
			t.Fatalf("remote path %q should fail", remote)
		}
	}
	if _, err := multiPullDestination(t.TempDir(), "../web", "/etc/hosts", false); err == nil {
		t.Fatal("unsafe alias should fail")
	}
}

func TestWindowsPathComponentValidation(t *testing.T) {
	for _, invalid := range []string{"CON", "con.txt", "bad:name", "trail.", "trail "} {
		if err := validateLocalComponent(invalid, true); err == nil {
			t.Fatalf("%q should fail Windows validation", invalid)
		}
	}
	if err := validateLocalComponent("nginx.conf", true); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureUniqueDestinationsDetectsFlatAndCaseCollisions(t *testing.T) {
	root := t.TempDir()
	same := filepath.Join(root, "hosts")
	if err := ensureUniqueDestinations([]string{same, same}, false); err == nil {
		t.Fatal("same destination should conflict")
	}
	err := ensureUniqueDestinations([]string{filepath.Join(root, "Web"), filepath.Join(root, "web")}, true)
	if err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRemoteManifestDestinationsRejectsUnsafeAndWindowsCollisions(t *testing.T) {
	root := t.TempDir()
	if err := validateRemoteManifestDestinations(root, []manifestEntry{
		{Path: ".", Type: "dir"},
		{Path: "safe/file", Type: "file"},
	}, false, false); err != nil {
		t.Fatal(err)
	}
	if err := validateRemoteManifestDestinations(root, []manifestEntry{{Path: `bad\name`, Type: "file"}}, false, false); err == nil {
		t.Fatal("unsafe local component should fail")
	}
	if err := validateRemoteManifestDestinations(root, []manifestEntry{
		{Path: "Web/file", Type: "file"},
		{Path: "web/file", Type: "file"},
	}, true, true); err == nil {
		t.Fatal("Windows case collision should fail")
	}
}

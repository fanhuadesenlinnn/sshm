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

func TestSinglePullDirectoryDestinationIsStableAcrossRuns(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "downloaded-tree")
	first, err := singlePullDestination("/var/lib/tree", destination, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	second, err := singlePullDestination("/var/lib/tree", destination, true)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first != destination {
		t.Fatalf("directory destination changed across runs: first=%q second=%q", first, second)
	}
}

func TestSinglePullUsesContainerOnlyWhenUnambiguous(t *testing.T) {
	root := t.TempDir()
	fileDestination, err := singlePullDestination("/var/log/app.log", root, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "app.log"); fileDestination != want {
		t.Fatalf("file destination = %q, want %q", fileDestination, want)
	}
	directoryDestination, err := singlePullDestination("/var/lib/tree", root+string(os.PathSeparator), true)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "tree"); directoryDestination != want {
		t.Fatalf("explicit container destination = %q, want %q", directoryDestination, want)
	}
}

func TestSinglePullDirectoryTreatsWorkingDirectoryAndAncestorsAsContainers(t *testing.T) {
	root := t.TempDir()
	working := filepath.Join(root, "work", "nested")
	if err := os.MkdirAll(working, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(working)

	for _, destination := range []string{".", root} {
		got, err := singlePullDestination("/var/lib/tree", destination, true)
		if err != nil {
			t.Fatal(err)
		}
		absolute, err := filepath.Abs(destination)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(absolute, "tree"); got != want {
			t.Fatalf("protected destination %q = %q, want %q", destination, got, want)
		}
	}
}

func TestActivateLocalTempRefusesToReplaceWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "work")
	temp := filepath.Join(root, "download")
	if err := os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "sentinel"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(temp, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(destination)
	if err := activateLocalTemp(temp, destination, true, true, false); err == nil || !strings.Contains(err.Error(), "受保护") {
		t.Fatalf("activateLocalTemp() error = %v, want protected-directory refusal", err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "sentinel")); err != nil || string(data) != "keep" {
		t.Fatalf("working directory was modified: data=%q err=%v", data, err)
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

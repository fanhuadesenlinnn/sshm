package command

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseTransferOptionsMapsPushAndPull(t *testing.T) {
	push, err := parseTransferOptions([]string{"one", "./local", "/remote", "--overwrite", "--yes"}, "push", false)
	if err != nil {
		t.Fatal(err)
	}
	if push.localPath != "./local" || push.remotePath != "/remote" || !push.overwrite || !push.batch.Yes || len(push.targets) != 1 {
		t.Fatalf("unexpected push options: %+v", push)
	}
	pull, err := parseTransferOptions([]string{"one", "/remote", "./local"}, "pull", false)
	if err != nil {
		t.Fatal(err)
	}
	if pull.remotePath != "/remote" || pull.localPath != "./local" {
		t.Fatalf("pull paths are reversed: %+v", pull)
	}
}

func TestParseTransferOptionsMapsVirtualAllTag(t *testing.T) {
	options, err := parseTransferOptions([]string{"all", "./local", "/remote"}, "push", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.targets) != 1 || options.targets[0] != "--all" {
		t.Fatalf("targets = %v", options.targets)
	}
}

func TestParseTransferOptionsRejectsConflictingOverwriteModes(t *testing.T) {
	if _, err := parseTransferOptions([]string{"one", "local", "/remote", "--overwrite", "--backup"}, "push", false); err == nil {
		t.Fatal("--overwrite and --backup should conflict")
	}
}

func TestConnectRejectsExtraOpenSSHArguments(t *testing.T) {
	app := NewApp()
	if err := app.cmdConnect([]string{"server", "-L", "8080:localhost:80"}); err == nil {
		t.Fatal("extra OpenSSH arguments should be rejected")
	}
}

func TestSafeRemoteEntryNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../escape", `..\\escape`, "a/b"} {
		if _, err := safeRemoteEntryName(name); err == nil {
			t.Fatalf("name %q should be rejected", name)
		}
	}
	if got, err := safeRemoteEntryName("it's safe.txt"); err != nil || got != "it's safe.txt" {
		t.Fatalf("safe name = %q, err = %v", got, err)
	}
}

func TestCleanTransferRemotePathRejectsUnsafePaths(t *testing.T) {
	for _, remote := range []string{"", ".", "/", "~", "~/dest", "../secret", "/etc/../secret", "safe/\x00path"} {
		if _, err := cleanTransferRemotePath(remote); err == nil {
			t.Fatalf("remote path %q should be rejected", remote)
		}
	}
	if got, err := cleanTransferRemotePath("folder with space/it's-ready"); err != nil || got != "folder with space/it's-ready" {
		t.Fatalf("safe remote path = %q, err = %v", got, err)
	}
}

func TestClassifyDiffDataDistinguishesTextAndBinary(t *testing.T) {
	if _, text, err := classifyDiffData([]byte("old\nnew\n")); err != nil || !text {
		t.Fatalf("text=%t err=%v", text, err)
	}
	if _, text, err := classifyDiffData([]byte{'a', 0, 'b'}); err != nil || text {
		t.Fatalf("binary text=%t err=%v", text, err)
	}
	var output bytes.Buffer
	for _, line := range splitDiffLines([]byte("one\ntwo\n")) {
		output.WriteString("+" + line + "\n")
	}
	if !strings.Contains(output.String(), "+one\n+two\n") {
		t.Fatalf("diff output = %q", output.String())
	}
}

func TestTransferRetryCommandUsesShellSafeQuotingAndPreservesMethod(t *testing.T) {
	got := transferRetryCommand(transferOptions{
		direction: "push", localPath: "dist/$(uname).tar.gz", remotePath: "/opt/app it's.tar.gz",
		method: "rsync", overwrite: true, validateChecksum: false,
	}, "web'01")
	for _, want := range []string{
		"sshm push 'web'\"'\"'01' 'dist/$(uname).tar.gz' '/opt/app it'\"'\"'s.tar.gz' --yes",
		"--overwrite",
		"--no-validate-checksum",
		"--method 'rsync'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retry command missing %q:\n%s", want, got)
		}
	}
}

func TestLocalManifestEntryRejectsSpecialFile(t *testing.T) {
	for name, mode := range map[string]os.FileMode{
		"fifo": os.ModeNamedPipe | 0600,
		"link": os.ModeSymlink | 0777,
	} {
		_, err := localManifestEntry("/tmp/"+name, ".", fakeFileInfo{name: name, mode: mode})
		if err == nil || !strings.Contains(err.Error(), "特殊文件") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}

type fakeFileInfo struct {
	name string
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

package command

import "testing"

func TestParseTransferOptionsMapsPushAndPull(t *testing.T) {
	push, err := parseTransferOptions([]string{"./local", "/remote", "one", "--overwrite", "--yes"}, "push")
	if err != nil {
		t.Fatal(err)
	}
	if push.localPath != "./local" || push.remotePath != "/remote" || !push.overwrite || !push.yes || len(push.targets) != 1 {
		t.Fatalf("unexpected push options: %+v", push)
	}
	pull, err := parseTransferOptions([]string{"/remote", "./local", "one"}, "pull")
	if err != nil {
		t.Fatal(err)
	}
	if pull.remotePath != "/remote" || pull.localPath != "./local" {
		t.Fatalf("pull paths are reversed: %+v", pull)
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

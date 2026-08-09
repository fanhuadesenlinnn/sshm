package command

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

func TestParseSSHConfig(t *testing.T) {
	content := `
# comment
Host web01 web02
    HostName 10.0.0.11
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519

Host db*
    HostName 10.0.0.12

Host jump
    HostName example.com
`
	hosts := parseSSHConfig(content)
	if len(hosts) != 3 {
		t.Fatalf("hosts = %+v", hosts)
	}
	byAlias := map[string]config.Host{}
	for _, host := range hosts {
		byAlias[host.Alias] = host
	}
	web01 := byAlias["web01"]
	if web01.Host != "10.0.0.11" || web01.User != "deploy" || web01.Port != 2222 ||
		web01.Identity != "~/.ssh/id_ed25519" {
		t.Fatalf("web01 = %+v", web01)
	}
	if web02 := byAlias["web02"]; web02.Host != "10.0.0.11" {
		t.Fatalf("web02 = %+v", web02)
	}
	if jump := byAlias["jump"]; jump.Host != "example.com" {
		t.Fatalf("jump = %+v", jump)
	}
}

func TestParseSSHConfigSkipsWildcardAndNoise(t *testing.T) {
	hosts := parseSSHConfig("Host db*\n  HostName 10.0.0.12\n\njunk line\n")
	for _, host := range hosts {
		if host.Alias == "db*" {
			t.Fatalf("通配符 Host 不应导入: %+v", hosts)
		}
	}
}

func TestImportSSHConfigKeepsHostWithIdentityAndPrintsManagedKeyFollowup(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshmd.yaml"))
	initCommandTestStore(t, store)
	sshConfig := filepath.Join(dir, "config")
	body := "Host web01\n  HostName 10.0.0.11\n  User deploy\n  Port 2222\n  IdentityFile ~/.ssh/id_ed25519\n"
	if err := os.WriteFile(sshConfig, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	var importErr error
	output := captureStdout(t, func() {
		importErr = app.cmdImportSSHConfig([]string{sshConfig})
	})
	if importErr != nil {
		t.Fatal(importErr)
	}
	host, _, _, err := store.FindHost("web01")
	if err != nil {
		t.Fatal(err)
	}
	if host.Host != "10.0.0.11" || host.User != "deploy" || host.Port != 2222 {
		t.Fatalf("host metadata was not imported: %+v", host)
	}
	if host.Identity != "" {
		t.Fatalf("unmanaged identity should be cleared: %+v", host)
	}
	for _, want := range []string{"identity 已清空", "sshmd key import", "~/.ssh/id_ed25519", "sshmd key use", "web01"} {
		if !strings.Contains(output, want) {
			t.Fatalf("follow-up output missing %q: %q", want, output)
		}
	}
}

func TestImportSSHConfigRejectsExtraArgumentsBeforeReading(t *testing.T) {
	app := &App{}
	if err := app.cmdImportSSHConfig([]string{"one", "two"}); err == nil || !strings.Contains(err.Error(), "用法") {
		t.Fatalf("extra argument should fail: %v", err)
	}
}

func TestExportSSHConfigWritesSafeFile(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshmd.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "web01", "deploy", "10.0.0.11", 2222
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ssh-config")
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdExportSSHConfig([]string{out}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"Host web01", "HostName 10.0.0.11", "User deploy", "Port 2222"} {
		if !containsText(text, want) {
			t.Fatalf("导出缺少 %q: %s", want, text)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("导出权限 = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestExportSSHConfigIncludesJumpAndIdentity(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshmd.yaml"))
	initCommandTestStore(t, store)
	bastion := config.DefaultHost()
	bastion.Alias, bastion.User, bastion.Host = "bastion", "root", "10.0.0.1"
	if err := store.Add(bastion); err != nil {
		t.Fatal(err)
	}
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "inner", "deploy", "10.0.0.11", 2222
	host.JumpHost = "bastion"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ssh-config")
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdExportSSHConfig([]string{out}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !containsText(text, "ProxyJump bastion") {
		t.Fatalf("导出缺少 ProxyJump: %s", text)
	}
}

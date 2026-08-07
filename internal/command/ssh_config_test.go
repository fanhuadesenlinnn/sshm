package command

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
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

func TestExportSSHConfigWritesSafeFile(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshm.yaml"))
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

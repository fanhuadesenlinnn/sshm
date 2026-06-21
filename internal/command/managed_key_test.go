package command

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
)

func TestManagedKeyCreateAndUse(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "sshm.yaml")
	hostStore := config.NewStoreWithPath(configPath)
	initCommandTestStore(t, hostStore)
	host := config.DefaultHost()
	host.Alias = "server"
	host.User = "root"
	host.Host = "example.com"
	if err := hostStore.Add(host); err != nil {
		t.Fatal(err)
	}
	secretPath := configPath
	app := &App{
		Store:       hostStore,
		Keys:        config.NewKeyStoreWithPath(configPath),
		ConfigPath:  secretPath,
		secretStore: secret.NewFileStore(secretPath, "master"),
	}
	if err := app.cmdKeyCreate([]string{"personal"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdKeyUse([]string{"personal", "server"}); err != nil {
		t.Fatal(err)
	}
	updated, _, _, err := hostStore.FindHost("server")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Identity != config.ManagedIdentity("personal") {
		t.Fatalf("Identity = %q", updated.Identity)
	}
	if _, err := app.secretStore.GetManagedKey("personal"); err != nil {
		t.Fatal(err)
	}
}

func TestSelectHostsSupportsTagAndAlias(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshm.yaml"))
	initCommandTestStore(t, store)
	for _, host := range []config.Host{
		{ID: config.NewID(), Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto", Tags: []string{"prod"}},
		{ID: config.NewID(), Alias: "two", User: "root", Host: "two", Port: 22, Auth: "auto", Tags: []string{"linux"}},
	} {
		if err := store.Add(host); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{Store: store}
	hosts, err := app.selectHosts([]string{"--tag", "prod", "--tag", "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("len(hosts) = %d, want 2", len(hosts))
	}
	hosts, err = app.selectHosts([]string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Alias != "one" {
		t.Fatalf("display ID selection = %+v", hosts)
	}
}

func TestInstallAndRevokeCommandsQuotePublicKey(t *testing.T) {
	publicKey := "ssh-ed25519 AAAA comment's-key"
	install := installPublicKeyCommand(publicKey)
	revoke := revokePublicKeyCommand(publicKey)
	for _, command := range []string{install, revoke} {
		if !strings.Contains(command, "'\"'\"'") {
			t.Fatalf("command does not safely quote apostrophe: %s", command)
		}
	}
	if !strings.Contains(install, "grep -qxF") || !strings.Contains(revoke, "grep -Fvx") {
		t.Fatal("commands are not idempotent/exact")
	}
	if !strings.Contains(revoke, "rc=$?") || !strings.Contains(revoke, "exit \"$rc\"") || strings.Contains(revoke, "|| true") {
		t.Fatalf("revoke command should preserve real grep/write failures: %s", revoke)
	}
}

func TestManagedKeyCreateAndImportRejectUnknownTrailingArgs(t *testing.T) {
	app := &App{}
	if err := app.cmdKeyCreate([]string{"personal", "--defualt"}); err == nil || !strings.Contains(err.Error(), "未知选项") {
		t.Fatalf("create unknown flag error = %v", err)
	}
	if err := app.cmdKeyImport([]string{"personal", "/tmp/id_ed25519", "extra"}); err == nil || !strings.Contains(err.Error(), "多余参数") {
		t.Fatalf("import extra arg error = %v", err)
	}
}

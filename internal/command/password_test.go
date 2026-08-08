package command

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
)

func TestCmdPasswdBatchRequiresConfirmation(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	initCommandTestStore(t, store)
	for _, alias := range []string{"one", "two"} {
		host := config.DefaultHost()
		host.Alias, host.User, host.Host, host.Tags = alias, "root", alias, []string{"prod"}
		if err := store.Add(host); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdPasswd([]string{"--tag", "prod"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("batch password without confirmation error = %v", err)
	}
}

func TestSavePasswordsForHostsUpdatesSelectedTargetsAtomically(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	initCommandTestStore(t, store)
	for _, host := range []config.Host{
		{ID: config.NewID(), Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto", Tags: []string{"prod"}},
		{ID: config.NewID(), Alias: "two", User: "root", Host: "two", Port: 22, Auth: "password", Tags: []string{"prod"}},
		{ID: config.NewID(), Alias: "three", User: "root", Host: "three", Port: 22, Auth: "auto"},
	} {
		if err := store.Add(host); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	hosts, err := app.selectHosts([]string{"--tag", "prod"})
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(store.Path(), "master")
	if err := savePasswordsForHosts(vault, hosts, "shared-password"); err != nil {
		t.Fatal(err)
	}

	for _, alias := range []string{"one", "two"} {
		host, _, _, err := store.FindHost(alias)
		if err != nil {
			t.Fatal(err)
		}
		if host.PasswordRef != host.ID {
			t.Fatalf("%s password ref = %q, want %q", alias, host.PasswordRef, host.ID)
		}
		password, err := vault.GetPassword(host.ID)
		if err != nil || password != "shared-password" {
			t.Fatalf("%s password = %q, err = %v", alias, password, err)
		}
	}
	untouched, _, _, err := store.FindHost("three")
	if err != nil {
		t.Fatal(err)
	}
	if untouched.PasswordRef != "" {
		t.Fatalf("unselected host password ref = %q", untouched.PasswordRef)
	}
}

func TestEncryptPlaintextPasswordsMovesToVault(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "one", "root", "one"
	host.Password = "plain-secret"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	selected, err := app.selectHosts([]string{"one"})
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(store.Path(), "master")
	if err := encryptPlaintextPasswords(vault, selected); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost("one")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Password != "" || loaded.PasswordRef == "" {
		t.Fatalf("升级后应为 vault 引用且清明文: %+v", loaded)
	}
	password, err := vault.GetPassword(loaded.PasswordRef)
	if err != nil || password != "plain-secret" {
		t.Fatalf("vault 密码 = %q, err = %v", password, err)
	}
}

func TestSavePasswordsForHostsRollsBackWhenTargetDisappears(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "one", "root", "one"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	vault := secret.NewFileStore(store.Path(), "master")
	if err := savePasswordsForHosts(vault, []config.Host{host}, "before"); err != nil {
		t.Fatal(err)
	}
	missing := config.DefaultHost()
	missing.Alias, missing.User, missing.Host = "missing", "root", "missing"
	if err := savePasswordsForHosts(vault, []config.Host{host, missing}, "after"); err == nil {
		t.Fatal("missing target should fail the entire password update")
	}
	password, err := vault.GetPassword(host.ID)
	if err != nil || password != "before" {
		t.Fatalf("password changed after failed batch: %q, err = %v", password, err)
	}
}

func TestRemovePasswordsForHostsClearsBatch(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	initCommandTestStore(t, store)
	var hosts []config.Host
	for _, alias := range []string{"one", "two"} {
		host := config.DefaultHost()
		host.Alias, host.User, host.Host, host.Auth = alias, "root", alias, "password"
		if err := store.Add(host); err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, host)
	}
	vault := secret.NewFileStore(store.Path(), "master")
	if err := savePasswordsForHosts(vault, hosts, "shared-password"); err != nil {
		t.Fatal(err)
	}
	if err := removePasswordsForHosts(vault, hosts); err != nil {
		t.Fatal(err)
	}
	for _, original := range hosts {
		host, _, _, err := store.FindHost(original.Alias)
		if err != nil {
			t.Fatal(err)
		}
		if host.PasswordRef != "" || host.Auth != "auto" {
			t.Fatalf("password state was not cleared for %s: %+v", host.Alias, host)
		}
		if _, err := vault.GetPassword(host.ID); err == nil {
			t.Fatalf("password entry still exists for %s", host.Alias)
		}
	}
}

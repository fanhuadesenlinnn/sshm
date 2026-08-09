package command

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
)

func TestDoctorFailsWhenConfiguredMasterPasswordCannotUnlockVault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHMD_HOME", home)
	configPath := filepath.Join(home, "sshmd.yaml")
	store := config.NewStoreWithPath(configPath)
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "prod", "root", "example.com"
	host.PasswordRef = host.ID
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	if err := secret.NewFileStore(configPath, "correct-master").SetPassword(host.ID, "ssh-password"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(masterPasswordEnv, "wrong-master")
	app := &App{Store: store, Keys: config.NewKeyStoreWithPath(configPath), ConfigPath: configPath}
	err := app.cmdDoctor(nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 4 {
		t.Fatalf("doctor should fail with the vault exit code, error = %v", err)
	}
}

func TestDoctorFailsForHostWithoutUsableCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SSHMD_HOME", home)
	configPath := filepath.Join(home, "sshmd.yaml")
	store := config.NewStoreWithPath(configPath)
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "prod", "root", "example.com"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, Keys: config.NewKeyStoreWithPath(configPath), ConfigPath: configPath}
	err := app.cmdDoctor(nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("doctor should fail with the configuration exit code, error = %v", err)
	}
}

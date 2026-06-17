package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setTestUserHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
	}
}

func TestResolvePathsUsesDotSSHMAndIgnoresLegacyOverrides(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	t.Setenv("SSHM_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("SSHM_CONFIG_FILE", filepath.Join(home, "ignored.yaml"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Home != filepath.Join(home, ".sshm") {
		t.Fatalf("home = %q", paths.Home)
	}
	if paths.Config != filepath.Join(home, ".sshm", "sshm.yaml") {
		t.Fatalf("config = %q", paths.Config)
	}
}

func TestResolvePathsUsesSSHMHomeForAllOwnedPaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "portable")
	t.Setenv("SSHM_HOME", home)
	paths, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"config": paths.Config, "logs": paths.Logs, "deploy": paths.Deploy,
		"deploy.d": paths.DeployDir, "backups": paths.Backups, "tmp": paths.Temp,
	} {
		if !strings.HasPrefix(path, home+string(os.PathSeparator)) {
			t.Fatalf("%s path escaped SSHM_HOME: %s", name, path)
		}
	}
}

func TestInitializeCreatesChineseV2ConfigAndForceBackup(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("SSHM_HOME", home)

	paths, backup, err := Initialize(false)
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatalf("unexpected backup: %s", backup)
	}
	data, err := os.ReadFile(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"# sshm 配置文件", "# 主机密钥策略:", "version: 2", "retention: 30d", "vault: null"} {
		if !strings.Contains(text, want) {
			t.Fatalf("default config missing %q:\n%s", want, text)
		}
	}
	for _, path := range []string{paths.Home, paths.DeployDir, paths.Logs, paths.Backups, paths.Temp} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}
	if _, _, err := Initialize(false); err == nil {
		t.Fatal("second init should refuse overwrite")
	}
	time.Sleep(time.Second)
	_, backup, err = Initialize(true)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("force init should create backup")
	}
	if info, err := os.Stat(backup); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatalf("backup stat = %v, %v", info, err)
	}
}

func TestExpandPathSupportsWindowsTildeSeparator(t *testing.T) {
	home := t.TempDir()
	setTestUserHome(t, home)
	got := ExpandPath(`~\keys\id_ed25519`)
	want := filepath.Join(home, "keys", "id_ed25519")
	if got != want {
		t.Fatalf("ExpandPath windows tilde = %q, want %q", got, want)
	}
}

package command

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestPingSingleHostUnreachableExitCode(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshm.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.Port = "dead", "root", "127.0.0.1", 1
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	err := app.cmdPing([]string{"dead"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("应返回 ExitError: %v", err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("单主机不可达退出码 = %d, want 2", exitErr.Code)
	}
}

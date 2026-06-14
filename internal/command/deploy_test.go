package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

func TestParseDeployCLIOptionsSupportsRepeatedFilesAndTargetOverride(t *testing.T) {
	options, err := parseDeployCLIOptions([]string{
		"install", "-f", "base.yaml", "--file", "project.yaml",
		"--host", "one,two", "--tag", "prod,web", "--mode", "visible", "--yes", "--output", "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.files) != 2 || len(options.positionals) != 1 || !options.hasSelector ||
		len(options.selector.Hosts) != 1 || len(options.selector.Tags) != 1 ||
		options.mode != "visible" || !options.yes || options.output != "json" {
		t.Fatalf("options = %+v", options)
	}
}

func TestDeployPlanAllowsRuntimeTargetForTargetlessProfile(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshm.yaml"))
	host := config.DefaultHost()
	host.Alias, host.User, host.Host = "one", "root", "127.0.0.1"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deploy.yaml")
	body := "version: 1\nprofiles:\n  - name: targetless\n    steps:\n      - {type: exec, command: hostname}\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdDeployPlan([]string{"targetless", "-f", path, "--host", "one", "--output", "json"}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdDeployValidate([]string{"-f", path}); err == nil {
		t.Fatal("validate should reject a targetless profile without a runtime override")
	}
}

func TestDeployInitRefusesOverwriteUnlessExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	app := NewApp()
	if err := app.cmdDeployInit([]string{"-f", path}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdDeployInit([]string{"-f", path}); err == nil {
		t.Fatal("expected existing file error")
	}
	if err := app.cmdDeployInit([]string{"-f", path, "--overwrite"}); err != nil {
		t.Fatal(err)
	}
}

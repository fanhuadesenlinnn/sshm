package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestParseDeployCLIOptionsSupportsV2RunFlags(t *testing.T) {
	options, err := parseDeployCLIOptions([]string{
		"install", "-f", "base.yaml", "--file", "project.yaml",
		"--host", "one,two", "--tag", "prod,web", "--serial", "2", "--parallel", "3",
		"--fail-fast", "--max-fail", "2", "--check", "--diff", "--yes", "--output", "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.files) != 2 || len(options.positionals) != 1 || !options.hasSelector ||
		len(options.selector.Hosts) != 1 || len(options.selector.Tags) != 1 ||
		options.batch.Serial != 2 || options.batch.Parallel != 3 || !options.batch.FailFast ||
		options.batch.MaxFail != 2 || !options.check || !options.diff || !options.batch.Yes || options.output != "json" {
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
	body := "version: 2\nprofiles:\n  - name: targetless\n    steps:\n      - exec: hostname\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.deployPlanCommand([]string{"targetless", "-f", path, "--host", "one", "--output", "json"}, false); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdDeployValidate([]string{"-f", path}); err == nil {
		t.Fatal("validate should reject a targetless profile without a runtime override")
	}
}

func TestDeployInitRefusesOverwriteUnlessExplicitAndWritesV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	app := NewApp()
	if err := app.cmdDeployInit([]string{"-f", path}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:10]) != "# sshm dep" || !containsText(string(data), "version: 2", "更新应用并重启服务") {
		t.Fatalf("sample = %s", data)
	}
	if err := app.cmdDeployInit([]string{"-f", path}); err == nil {
		t.Fatal("expected existing file error")
	}
	if err := app.cmdDeployInit([]string{"-f", path, "--overwrite"}); err != nil {
		t.Fatal(err)
	}
}

func TestDeployValidateAllowsInitializedSampleBeforeHosts(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "sshm.yaml"))
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deploy.yaml")
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdDeployInit([]string{"-f", path}); err != nil {
		t.Fatal(err)
	}
	if err := app.cmdDeployValidate([]string{"-f", path}); err != nil {
		t.Fatalf("sample should validate before hosts are added: %v", err)
	}
	if err := app.cmdDeployList([]string{"-f", path}); err != nil {
		t.Fatalf("sample should list before hosts are added: %v", err)
	}
	err := app.deployPlanCommand([]string{"update-app", "-f", path}, false)
	if err == nil || !strings.Contains(err.Error(), "还没有主机") {
		t.Fatalf("plan without hosts should give next-step guidance: %v", err)
	}
}

func containsText(value string, values ...string) bool {
	for _, item := range values {
		if !strings.Contains(value, item) {
			return false
		}
	}
	return true
}

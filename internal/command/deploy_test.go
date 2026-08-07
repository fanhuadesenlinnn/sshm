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
	initCommandTestStore(t, store)
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

func TestDeployInitRefusesOverwriteUnlessExplicitAndWritesV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	app := NewApp()
	var err error
	output := captureStdout(t, func() {
		err = app.cmdDeployInit([]string{"-f", path})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "sshm deploy validate -f") || !strings.Contains(output, "sshm deploy plan update-app -f") {
		t.Fatalf("explicit deploy init should print next steps with -f path: %q", output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# sshm Deploy v3") || !containsText(
		string(data),
		"version: 3",
		"plays: []",
		"更新应用并重启服务",
		"hosts 列表 / tags 标签 / all",
		"请勿在本文件中保存密码、私钥",
	) {
		t.Fatalf("sample = %s", data)
	}
	v2Path := filepath.Join(t.TempDir(), "deploy-v2.yaml")
	if err := app.cmdDeployInit([]string{"-f", v2Path, "--version", "2"}); err != nil {
		t.Fatal(err)
	}
	v2Data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(v2Data), "# sshm Deploy v2") || !strings.Contains(string(v2Data), "version: 2") {
		t.Fatalf("--version 2 sample = %s", v2Data)
	}
	if err := app.cmdDeployInit([]string{"-f", v2Path, "--version", "4"}); err == nil {
		t.Fatal("--version 4 应当报错")
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
	if err == nil || !strings.Contains(err.Error(), "未找到 deploy play") {
		t.Fatalf("plan of commented sample should report missing play: %v", err)
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

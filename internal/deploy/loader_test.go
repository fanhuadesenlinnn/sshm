package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

func TestDiscoverAndLoadMultipleFiles(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("SSHM_HOME", home)
	writeDeployTestFile(t, filepath.Join(home, "deploy.yaml"), deployTestYAML("global", "prod"))
	writeDeployTestFile(t, filepath.Join(home, "deploy.d", "20-second.yaml"), deployTestYAML("second", "prod"))
	writeDeployTestFile(t, filepath.Join(home, "deploy.d", "10-first.yaml"), deployTestYAML("first", "prod"))
	writeDeployTestFile(t, filepath.Join(cwd, "sshm.deploy.yaml"), deployTestYAML("local", "prod"))

	paths, err := Discover(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, "\n"); !strings.Contains(got, "deploy.yaml\n") ||
		strings.Index(got, "10-first.yaml") > strings.Index(got, "20-second.yaml") ||
		!strings.HasSuffix(got, "sshm.deploy.yaml") {
		t.Fatalf("unexpected discovery order:\n%s", got)
	}
	catalog, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Profiles) != 4 {
		t.Fatalf("profiles = %d", len(catalog.Profiles))
	}
}

func TestLoadRejectsDuplicateProfilesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.yaml")
	two := filepath.Join(dir, "two.yaml")
	writeDeployTestFile(t, one, deployTestYAML("same", "prod"))
	writeDeployTestFile(t, two, deployTestYAML("same", "prod"))
	if _, err := Load([]string{one, two}); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate profile error = %v", err)
	}
}

func TestResolveStepsUsesFileDirectoryAndSinglePassVars(t *testing.T) {
	profile := Profile{
		BaseDir: "/project/tasks",
		Vars:    map[string]string{"package": "./dist", "dest": "/opt/app"},
		Steps: []Step{
			{Type: "copy", Src: "${package}", Dest: "${dest}"},
			{Type: "exec", Command: "test -d ${dest}"},
		},
	}
	steps, err := ResolveSteps(profile)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].Src != "/project/tasks/dist" || steps[0].Method != "auto" || steps[1].Command != "test -d /opt/app" {
		t.Fatalf("resolved steps = %+v", steps)
	}
	profile.Vars["dest"] = "${other}"
	if err := ValidateProfile(Profile{
		Name: "bad", Vars: profile.Vars, Targets: TargetSelector{All: true},
		Strategy: applyStrategyDefaults(Strategy{}), Steps: []Step{{Type: "exec", Command: "echo ok"}},
	}, []config.Host{{Alias: "one"}}, false); err == nil || !strings.Contains(err.Error(), "不能递归") {
		t.Fatalf("recursive var error = %v", err)
	}
}

func TestResolveStepsRejectsEmptyRequiredValueAfterExpansion(t *testing.T) {
	for _, step := range []Step{
		{Type: "exec", Command: "${empty}"},
		{Type: "copy", Src: "${empty}", Dest: "/tmp/app"},
	} {
		if _, err := ResolveSteps(Profile{Vars: map[string]string{"empty": ""}, Steps: []Step{step}}); err == nil {
			t.Fatalf("step should fail after empty expansion: %+v", step)
		}
	}
}

func TestVisibleModeDefaultsToSerial(t *testing.T) {
	strategy := mergeStrategy(Strategy{MaxParallel: 5}, Strategy{Mode: "visible"})
	if strategy.MaxParallel != 1 {
		t.Fatalf("visible parallel = %d", strategy.MaxParallel)
	}
	if err := validateStrategy(strategy); err != nil {
		t.Fatal(err)
	}
}

func TestProfileCanOverrideDefaultRetryCountWithZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeDeployTestFile(t, path, `version: 1
defaults:
  strategy:
    retry_count: 3
profiles:
  - name: no-retry
    targets: {all: true}
    strategy:
      retry_count: 0
    steps:
      - {type: exec, command: hostname}
`)
	catalog, err := Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Profiles[0].Strategy.RetryCount != 0 {
		t.Fatalf("retry count = %d", catalog.Profiles[0].Strategy.RetryCount)
	}
}

func TestLoadRejectsUnknownStrategyField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeDeployTestFile(t, path, `version: 1
defaults:
  strategy:
    made_up: true
profiles:
  - name: test
    targets: {all: true}
    steps: [{type: exec, command: hostname}]
`)
	if _, err := Load([]string{path}); err == nil || !strings.Contains(err.Error(), "未知 strategy 字段") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestLoadPreservesExplicitInvalidZeroParallelForValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeDeployTestFile(t, path, `version: 1
profiles:
  - name: test
    targets: {all: true}
    strategy: {max_parallel: 0}
    steps: [{type: exec, command: hostname}]
`)
	catalog, err := Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(catalog, []config.Host{{Alias: "one"}}); err == nil || !strings.Contains(err.Error(), "max_parallel") {
		t.Fatalf("zero parallel error = %v", err)
	}
}

func writeDeployTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func deployTestYAML(name, tag string) string {
	return "version: 1\nprofiles:\n  - name: " + name + "\n    targets:\n      tags: [" + tag + "]\n    steps:\n      - type: exec\n        command: hostname\n"
}

package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestDiscoverUsesUserFilesAndNeverImplicitCWD(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("SSHM_HOME", home)
	writeDeployTestFile(t, filepath.Join(home, "deploy.yaml"), deployTestYAML("global", "prod"))
	writeDeployTestFile(t, filepath.Join(home, "deploy.d", "20-second.yaml"), deployTestYAML("second", "prod"))
	writeDeployTestFile(t, filepath.Join(home, "deploy.d", "10-first.yaml"), deployTestYAML("first", "prod"))
	local := filepath.Join(cwd, "sshm.deploy.yaml")
	writeDeployTestFile(t, local, deployTestYAML("local", "prod"))

	paths, err := Discover(nil, cwd)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(paths, "\n")
	if strings.Contains(got, local) || strings.Index(got, "10-first.yaml") > strings.Index(got, "20-second.yaml") {
		t.Fatalf("unexpected discovery order:\n%s", got)
	}

	explicit, err := Discover([]string{local}, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) != 1 || explicit[0] != local {
		t.Fatalf("explicit paths = %v", explicit)
	}
}

func TestLoadStrictV2AndGlobalNames(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.yaml")
	two := filepath.Join(dir, "two.yaml")
	writeDeployTestFile(t, one, deployTestYAML("same", "prod"))
	writeDeployTestFile(t, two, deployTestYAML("same", "prod"))
	if _, err := Load([]string{one, two}); err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate profile error = %v", err)
	}

	writeDeployTestFile(t, one, "version: 1\nprofiles: []\n")
	if _, err := Load([]string{one}); err == nil || !strings.Contains(err.Error(), "仅支持 2") {
		t.Fatalf("version error = %v", err)
	}
	writeDeployTestFile(t, one, "version: 2\nunknown: true\nprofiles: []\n")
	if _, err := Load([]string{one}); err == nil || !strings.Contains(err.Error(), "field unknown") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestValidateV2ActionsHandlersAndNotify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	writeDeployTestFile(t, path, `version: 2
profiles:
  - name: app
    targets: {all: true}
    steps:
      - name: invalid
        exec: hostname
        mkdir: {path: /tmp/app}
`)
	catalog, err := Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(catalog, []config.Host{{Alias: "one"}}); err == nil || !strings.Contains(err.Error(), "只能包含一个 action") {
		t.Fatalf("action error = %v", err)
	}

	writeDeployTestFile(t, path, `version: 2
profiles:
  - name: app
    targets: {all: true}
    steps:
      - name: upload
        push: {src: ./app, dest: /opt/app}
        notify: [restart]
handlers:
  - name: reload
    exec: systemctl reload app
`)
	catalog, err = Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalog(catalog, []config.Host{{Alias: "one"}}); err == nil || !strings.Contains(err.Error(), "不存在的 handler") {
		t.Fatalf("notify error = %v", err)
	}
}

func TestResolveStepsUsesDeclaringFileDirectory(t *testing.T) {
	baseDir := filepath.FromSlash("/project/tasks")
	profile := Profile{
		Name: "app", BaseDir: baseDir, Targets: TargetSelector{All: true},
		Steps: []Step{
			{Name: "push", Push: &PushAction{Src: "./dist/app", Dest: "/opt/app"}},
			{Name: "pull", Pull: &PullAction{Src: "/var/log/app.log", Dest: "./logs"}},
		},
	}
	steps, err := ResolveSteps(profile)
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].Push.Src != filepath.Join(baseDir, "dist", "app") || steps[1].Pull.Dest != filepath.Join(baseDir, "logs") {
		t.Fatalf("resolved steps = %+v", steps)
	}
}

func TestValidateRejectsUnsafeRemotePaths(t *testing.T) {
	for _, step := range []Step{
		{Push: &PushAction{Src: "/tmp/src", Dest: "~/dest"}},
		{Pull: &PullAction{Src: "../secret", Dest: "/tmp/dest"}},
		{Mkdir: &MkdirAction{Path: "/"}},
	} {
		if err := validateStep(step, false); err == nil {
			t.Fatalf("unsafe step should fail: %+v", step)
		}
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
	return "version: 2\nprofiles:\n  - name: " + name + "\n    targets:\n      tags: [" + tag + "]\n    steps:\n      - exec: hostname\n"
}

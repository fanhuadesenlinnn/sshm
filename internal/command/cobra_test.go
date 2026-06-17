package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

func TestCobraHelpWorksWithoutInitializationAndRemovedCommandsAreAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	app := &App{Store: config.NewStoreWithPath(path), ConfigPath: path}
	for _, args := range [][]string{{"--help"}, {"deploy", "--help"}, {"deploy", "run", "--help"}} {
		if err := runCobra(app, args); err != nil {
			t.Fatalf("help %v: %v", args, err)
		}
	}
	root := newRootCommand(app)
	for _, name := range []string{"exec-all", "push-all", "pull-all"} {
		if knownRootCommand(root, name) {
			t.Fatalf("removed command %s is still registered", name)
		}
	}
}

func TestDoctorWorksWithoutInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	app := &App{Store: config.NewStoreWithPath(path), ConfigPath: path}
	if err := app.cmdDoctor(nil); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyDocumentedCommandsRunWithoutInitialization(t *testing.T) {
	t.Setenv("SSHM_HOME", t.TempDir())
	path := config.ConfigFilePath()
	app := &App{Store: config.NewStoreWithPath(path), ConfigPath: path}
	for _, args := range [][]string{{"init"}, {"config", "path"}, {"doctor"}} {
		if err := runCobra(app, args); err != nil {
			t.Fatalf("allowed command %v: %v", args, err)
		}
		if args[0] == "init" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, args := range [][]string{{"list"}, {"deploy", "init", "--stdout"}} {
		if err := runCobra(app, args); !errors.Is(err, config.ErrNotInitialized) {
			t.Fatalf("command %v should require initialization: %v", args, err)
		}
	}
	if err := runCobra(app, []string{"completion", "bash"}); err != nil {
		t.Fatalf("completion script should not require initialization: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("commands unexpectedly created config: %v", err)
	}
}

func TestCompletionCandidatesMatchV6CommandSurface(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: path}
	candidates, err := app.completionCandidates()
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(candidates, "\n") + "\n"
	for _, name := range []string{"init", "config", "doctor", "deploy", "exec-tag", "push-tag", "pull-tag"} {
		if !strings.Contains(joined, "\n"+name+"\n") {
			t.Fatalf("missing completion candidate %q", name)
		}
	}
	for _, name := range []string{"exec-all", "push-all", "pull-all"} {
		if strings.Contains(joined, "\n"+name+"\n") {
			t.Fatalf("removed command is still a completion candidate: %q", name)
		}
	}
}

func TestCoreCommandHelpIncludesRunnableExamples(t *testing.T) {
	root := newRootCommand(&App{})
	if !strings.Contains(root.Example, "sshm init") || !strings.Contains(root.Example, "sshm passwd web01") {
		t.Fatalf("root examples should include first-run path: %q", root.Example)
	}
	cases := []struct {
		path        []string
		wantUse     string
		wantExample string
	}{
		{[]string{"exec"}, "exec [--yes] [--quiet] [--no-log] <别名|ID> <命令>", "sshm exec --yes web01"},
		{[]string{"exec-tag"}, "exec-tag [批量选项] <标签|all> <命令>", "sshm exec-tag all"},
		{[]string{"push"}, "push [选项] <别名|ID> <本地路径> <远程路径>", "sshm push web01"},
		{[]string{"key"}, "key <命令> [参数]", "sshm key setup personal web01 --yes"},
		{[]string{"tag"}, "tag <命令> [参数]", "sshm tag add prod"},
		{[]string{"deploy", "run"}, "run <profile>", "sshm deploy run webapp --tag prod"},
	}
	for _, tt := range cases {
		cmd, _, err := root.Find(tt.path)
		if err != nil {
			t.Fatalf("find %v: %v", tt.path, err)
		}
		if !strings.Contains(cmd.Use, tt.wantUse) {
			t.Fatalf("%v use = %q, want %q", tt.path, cmd.Use, tt.wantUse)
		}
		if !strings.Contains(cmd.Example, tt.wantExample) {
			t.Fatalf("%v example = %q, want to contain %q", tt.path, cmd.Example, tt.wantExample)
		}
	}
}

func TestCenterCommandsTreatNestedHelpAsHelp(t *testing.T) {
	app := &App{}
	for _, tt := range []struct {
		name string
		run  func() error
	}{
		{"key setup help", func() error { return app.cmdKey([]string{"setup", "--help"}) }},
		{"tag add help", func() error { return app.cmdTag([]string{"add", "--help"}) }},
		{"host add help", func() error { return app.cmdHost([]string{"add", "--help"}) }},
	} {
		if err := tt.run(); err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
	}
}

func TestUnknownCommandOrHostSuggestsClosestRootCommand(t *testing.T) {
	root := newRootCommand(&App{})
	cause := errors.New("missing host")
	err := unknownCommandOrHostError(root, "lst", cause)
	if err == nil || !strings.Contains(err.Error(), `"list"`) {
		t.Fatalf("unexpected suggestion: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error should wrap cause: %v", err)
	}
}

func TestExitCodeForErrorContract(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{errors.New("参数无效"), 3},
		{operation.Wrap(operation.StageNetwork, errors.New("down")), 2},
		{operation.Wrap(operation.StageExecute, errors.New("rc=1")), 1},
		{&ExitError{Code: 4, Err: errors.New("vault")}, 4},
		{context.Canceled, 130},
	}
	for _, tt := range cases {
		if got := ExitCodeForError(tt.err); got != tt.code {
			t.Fatalf("error %v: code=%d want=%d", tt.err, got, tt.code)
		}
	}
}

func TestConfigInsecureYesWorksThroughCobra(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: path}
	if err := runCobra(app, []string{"config", "host-key-policy", "insecure", "--yes"}); err != nil {
		t.Fatalf("config insecure --yes through cobra: %v", err)
	}
}

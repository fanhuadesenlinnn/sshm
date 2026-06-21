package command

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func TestParseSSHTarget(t *testing.T) {
	tests := []struct {
		input string
		user  string
		host  string
		port  int
	}{
		{"example.com", "root", "example.com", 22},
		{"deploy@example.com", "deploy", "example.com", 22},
		{"deploy@example.com:2222", "deploy", "example.com", 2222},
		{"root@[2001:db8::1]:2200", "root", "2001:db8::1", 2200},
	}
	for _, tt := range tests {
		user, host, port, err := parseSSHTarget(tt.input)
		if err != nil {
			t.Fatalf("parseSSHTarget(%q): %v", tt.input, err)
		}
		if user != tt.user || host != tt.host || port != tt.port {
			t.Fatalf("parseSSHTarget(%q) = %s, %s, %d", tt.input, user, host, port)
		}
	}
	if _, _, _, err := parseSSHTarget("root@example.com:not-a-port"); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}

func TestMatchHostTerms(t *testing.T) {
	h := config.Host{
		Alias: "prod-web",
		User:  "deploy",
		Host:  "10.0.0.1",
		Tags:  []string{"nginx", "public"},
		Note:  "main site",
	}
	if !matchHostTerms(h, []string{"web", "tag:nginx"}) {
		t.Fatal("expected all search terms to match")
	}
	if matchHostTerms(h, []string{"web", "tag:database"}) {
		t.Fatal("unexpected tag match")
	}
}

func TestSplitIndexedHostsPreservesDisplayedIDs(t *testing.T) {
	rows := []indexedHost{
		{host: config.Host{Alias: "b"}, index: 4},
		{host: config.Host{Alias: "a"}, index: 1},
	}
	hosts, indices := splitIndexedHosts(rows)
	if !reflect.DeepEqual(indices, []int{4, 1}) {
		t.Fatalf("indices = %v", indices)
	}
	if hosts[0].Alias != "b" || hosts[1].Alias != "a" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestListViewFlagsAreMutuallyExclusive(t *testing.T) {
	app := &App{}
	err := app.cmdList([]string{"--compact", "--wide"})
	if err == nil || !strings.Contains(err.Error(), "不能同时使用") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnknownOptionSuggestsClosestCommand(t *testing.T) {
	err := unknownOptionError("--lis")
	if err == nil || !strings.Contains(err.Error(), "--list") {
		t.Fatalf("unexpected suggestion: %v", err)
	}
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		injected string
		module   string
		want     string
	}{
		{injected: "v4.0.1", module: "v4.0.0", want: "v4.0.1"},
		{injected: "dev", module: "v4.0.1", want: "v4.0.1"},
		{injected: "dev", module: "(devel)", want: "dev"},
		{module: "", want: "dev"},
	}
	for _, tt := range tests {
		if got := resolveVersion(tt.injected, tt.module); got != tt.want {
			t.Fatalf("resolveVersion(%q, %q) = %q, want %q", tt.injected, tt.module, got, tt.want)
		}
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script := completionScript(shell)
		if !strings.Contains(script, "sshm completion candidates") {
			t.Fatalf("%s completion script is incomplete: %q", shell, script)
		}
	}
}

func TestCmdQuickAddPersistsDefaultsAndOptions(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	initCommandTestStore(t, store)
	app := &App{Store: store, ConfigPath: store.Path()}
	err := app.cmdQuickAdd([]string{
		"prod", "deploy@example.com:2222",
		"--tags", "web,linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, _, _, err := store.FindHost("prod")
	if err != nil {
		t.Fatal(err)
	}
	if host.User != "deploy" || host.Host != "example.com" || host.Port != 2222 {
		t.Fatalf("unexpected target: %+v", host)
	}
	if host.Auth != "auto" || !reflect.DeepEqual(host.Tags, []string{"web", "linux"}) {
		t.Fatalf("unexpected options: %+v", host)
	}
}

func TestCompletionCandidatesIncludeCommandsAndHosts(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias = "my-server"
	host.User = "root"
	host.Host = "example.com"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	if err := store.Repository().Update(func(doc *config.Document) error {
		doc.Tags.Ensure("production")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store}
	candidates, err := app.completionCandidates()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(candidates, " ")
	if !strings.Contains(joined, "my-server") || !strings.Contains(joined, "pick") || !strings.Contains(joined, "production") {
		t.Fatalf("unexpected candidates: %v", candidates)
	}
}

func TestCompletionCandidatesWorkWithoutInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	app := &App{Store: config.NewStoreWithPath(path), ConfigPath: path}
	candidates, err := app.completionCandidates()
	if err != nil {
		t.Fatal(err)
	}
	joined := "\n" + strings.Join(candidates, "\n") + "\n"
	for _, name := range []string{"init", "list", "host", "key", "tag", "deploy"} {
		if !strings.Contains(joined, "\n"+name+"\n") {
			t.Fatalf("missing completion candidate %q in %v", name, candidates)
		}
	}
}

func TestConfigEditRequiresInitializedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	app := &App{Store: store, ConfigPath: path}
	t.Setenv("EDITOR", "true")
	if err := app.cmdConfigEdit(nil); err == nil || !strings.Contains(err.Error(), "sshm init") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigInsecurePolicyRequiresConfirmationUnlessYes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: path}
	if err := app.cmdConfig([]string{"host-key-policy", "insecure"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("insecure policy without --yes should fail in non-interactive tests: %v", err)
	}
	if err := app.cmdConfig([]string{"host-key-policy", "insecure", "--yes"}); err != nil {
		t.Fatalf("insecure policy with --yes: %v", err)
	}
}

func TestSplitEditorCommandKeepsQuotedWindowsPath(t *testing.T) {
	parts := splitEditorCommand(`"C:\Program Files\Editor\editor.exe" --wait`)
	want := []string{`C:\Program Files\Editor\editor.exe`, "--wait"}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts = %#v, want %#v", parts, want)
	}
}

func TestParseExecArgsAcceptsTrailingOperationFlags(t *testing.T) {
	yes, quiet, noLog, host, command, err := parseExecArgs([]string{"web01", "uptime", "--yes", "--quiet", "--no-log"})
	if err != nil {
		t.Fatal(err)
	}
	if !yes || !quiet || !noLog || host != "web01" || command != "uptime" {
		t.Fatalf("unexpected parse result: yes=%t quiet=%t noLog=%t host=%q command=%q", yes, quiet, noLog, host, command)
	}
	yes, quiet, noLog, host, command, err = parseExecArgs([]string{"--yes", "web01", "--", "deploy", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if !yes || quiet || noLog || host != "web01" || command != "deploy --yes" {
		t.Fatalf("unexpected -- parse result: yes=%t quiet=%t noLog=%t host=%q command=%q", yes, quiet, noLog, host, command)
	}
}

func TestHostSelectorRejectsAllMixedWithSpecificTargets(t *testing.T) {
	for _, args := range [][]string{{"--all", "web01"}, {"--all", "--tag", "prod"}} {
		if _, err := parseHostSelector(args); err == nil || !strings.Contains(err.Error(), "--all") {
			t.Fatalf("parseHostSelector(%v) should reject mixed all selector: %v", args, err)
		}
	}
}

func TestLogsRejectUnknownSubcommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	if err := store.Repository().Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: path}
	if err := app.cmdLogs([]string{"unknown"}); err == nil || !strings.Contains(err.Error(), "未知 logs 命令") {
		t.Fatalf("logs unknown should fail: %v", err)
	}
}

func TestLogsCleanRejectsRootSSHMHome(t *testing.T) {
	t.Setenv("SSHM_HOME", string(os.PathSeparator))
	if _, err := safeLogsDirForClean(); err == nil || !strings.Contains(err.Error(), "根目录") {
		t.Fatalf("root SSHM_HOME should be rejected: %v", err)
	}
}

func TestDeleteRequiresConfirmationUnlessYes(t *testing.T) {
	store := config.NewStoreWithPath(filepath.Join(t.TempDir(), "sshm.yaml"))
	initCommandTestStore(t, store)
	host := config.DefaultHost()
	host.Alias = "prod"
	host.User = "root"
	host.Host = "example.com"
	if err := store.Add(host); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, ConfigPath: store.Path()}
	if err := app.cmdDelete([]string{"prod"}); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("delete without --yes in non-interactive test should fail: %v", err)
	}
	if err := app.cmdDelete([]string{"prod", "--yes"}); err != nil {
		t.Fatalf("delete with --yes: %v", err)
	}
	if _, _, _, err := store.FindHost("prod"); err == nil {
		t.Fatal("host should be deleted")
	}
}

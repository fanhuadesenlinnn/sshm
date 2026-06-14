package command

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
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

func TestConfigEditInitializesMissingSingleConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := config.NewStoreWithPath(path)
	app := &App{Store: store, ConfigPath: path}
	t.Setenv("EDITOR", "true")
	if err := app.cmdConfigEdit(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := config.NewRepositoryWithPath(path).Load(); err != nil {
		t.Fatal(err)
	}
}

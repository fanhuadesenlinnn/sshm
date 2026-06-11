package command

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"hello", []string{"hello"}},
		{"hello world", []string{"hello", "world"}},
		{"cmd arg1 arg2", []string{"cmd", "arg1", "arg2"}},
		{`cmd "arg with spaces"`, []string{"cmd", "arg with spaces"}},
		{`cmd 'single quoted'`, []string{"cmd", "single quoted"}},
		{"  leading space", []string{"leading", "space"}},
		{"trailing space  ", []string{"trailing", "space"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
	}

	for _, tt := range tests {
		got := parseArgs(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMigratePasswordRefsUsesStableID(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "hosts.yaml"))
	h := config.DefaultHost()
	h.Alias = "old-alias"
	h.User = "root"
	h.Host = "example.com"
	h.PasswordRef = h.Alias
	if err := store.Add(h); err != nil {
		t.Fatal(err)
	}

	secretPath := filepath.Join(dir, "secrets.yaml")
	fs := secret.NewFileStore(secretPath, "master")
	if err := fs.SetPassword(h.Alias, "secret"); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, SecretPath: secretPath, secretStore: fs}
	if err := app.migratePasswordRefs(fs); err != nil {
		t.Fatal(err)
	}

	loaded, _, _, err := store.FindHost(h.Alias)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PasswordRef != h.ID {
		t.Fatalf("PasswordRef = %q, want %q", loaded.PasswordRef, h.ID)
	}
	if got, err := fs.GetPassword(h.ID); err != nil || got != "secret" {
		t.Fatalf("GetPassword(stable ID) = %q, %v", got, err)
	}
	if _, err := fs.GetPassword(h.Alias); err == nil {
		t.Fatal("old alias password reference was not removed")
	}
}

func TestMigratePasswordRefsLeavesMissingSourceUnchanged(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStoreWithPath(filepath.Join(dir, "hosts.yaml"))
	h := config.DefaultHost()
	h.Alias = "missing-secret"
	h.User = "root"
	h.Host = "example.com"
	h.PasswordRef = "missing-old-ref"
	if err := store.Add(h); err != nil {
		t.Fatal(err)
	}
	fs := secret.NewFileStore(filepath.Join(dir, "secrets.yaml"), "master")
	if err := fs.SetPassword("other", "secret"); err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, SecretPath: filepath.Join(dir, "secrets.yaml"), secretStore: fs}
	if err := app.migratePasswordRefs(fs); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost(h.Alias)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PasswordRef != "missing-old-ref" {
		t.Fatalf("PasswordRef = %q, want missing-old-ref", loaded.PasswordRef)
	}
}

func TestMatchHost(t *testing.T) {
	h := config.Host{
		Alias: "webserver",
		Host:  "10.0.0.1",
		User:  "admin",
		Note:  "production web server",
		Group: "prod",
		Tags:  []string{"web", "nginx"},
	}

	tests := []struct {
		keyword string
		want    bool
	}{
		{"web", true},        // matches alias
		{"server", true},     // matches alias
		{"10.0", true},       // matches host
		{"admin", true},      // matches user
		{"production", true}, // matches note
		{"prod", true},       // matches group
		{"nginx", true},      // matches tag
		{"nonexistent", false},
		{"database", false},
		{"", true}, // empty keyword matches everything (contains check)
	}

	for _, tt := range tests {
		got := matchHost(h, tt.keyword)
		if got != tt.want {
			t.Errorf("matchHost(host, %q) = %v, want %v", tt.keyword, got, tt.want)
		}
	}
}

func TestParseSSHConfigMultipleAliases(t *testing.T) {
	hosts := parseSSHConfig(`
Host one two *.example
    HostName example.com
    User deploy
    Port 2222
    IdentityFile "~/.ssh/id key"
`)
	if len(hosts) != 2 {
		t.Fatalf("len(hosts) = %d, want 2", len(hosts))
	}
	for _, h := range hosts {
		if h.Host != "example.com" || h.User != "deploy" || h.Port != 2222 {
			t.Fatalf("unexpected host: %+v", h)
		}
	}
}

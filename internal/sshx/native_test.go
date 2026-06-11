package sshx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sshm/sshm/internal/config"
)

func TestGetAuthStrategy(t *testing.T) {
	tests := []struct {
		input string
		want  AuthStrategy
	}{
		{"auto", AuthAuto},
		{"key", AuthKey},
		{"password", AuthPassword},
		{"ask", AuthAsk},
		{"system", AuthSystem},
		{"", AuthAuto},
		{"unknown", AuthAuto},
	}

	for _, tt := range tests {
		got := GetAuthStrategy(tt.input)
		if got != tt.want {
			t.Errorf("GetAuthStrategy(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestKnownHostsPath(t *testing.T) {
	path := knownHostsPath()
	if path == "" {
		t.Fatal("knownHostsPath() returned empty string")
	}
	if filepath.Base(path) != "known_hosts" {
		t.Fatalf("knownHostsPath() = %q, want path ending with known_hosts", path)
	}
}

func TestEnsureSSHDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	if err := ensureSSHDir(); err != nil {
		t.Fatalf("ensureSSHDir() error = %v", err)
	}

	sshDir := filepath.Join(tmpDir, ".ssh")
	info, err := os.Stat(sshDir)
	if err != nil {
		t.Fatalf(".ssh dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".ssh is not a directory")
	}
}

func TestCreateHostKeyCallbackNoKnownHosts(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	cb, err := createHostKeyCallback()
	if err != nil {
		t.Fatalf("createHostKeyCallback() error = %v", err)
	}
	if cb == nil {
		t.Fatal("createHostKeyCallback() returned nil")
	}
}

func TestHasIdentityAbsent(t *testing.T) {
	if HasIdentity(config.Host{Alias: "test"}) {
		t.Fatal("HasIdentity() should be false for empty identity")
	}
}

func TestAuthStrategyConsts(t *testing.T) {
	seen := map[AuthStrategy]bool{}
	for _, s := range []AuthStrategy{AuthAuto, AuthKey, AuthPassword, AuthAsk, AuthSystem} {
		if seen[s] {
			t.Errorf("Duplicate AuthStrategy value: %v", s)
		}
		seen[s] = true
	}
}

func TestNativePingInvalidHost(t *testing.T) {
	ok, msg := NativePing(config.Host{Alias: "test", Host: "255.255.255.255", Port: 22, User: "nobody"}, "wrong-password")
	if ok {
		t.Fatalf("NativePing to invalid host should fail, got ok=true, msg=%q", msg)
	}
	if msg == "" {
		t.Fatal("NativePing should return an error message for invalid host")
	}
}

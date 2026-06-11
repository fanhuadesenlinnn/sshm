package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"golang.org/x/crypto/ssh"
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

func TestCreateHostKeyCallbackRejectsCorruptKnownHosts(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	if err := os.MkdirAll(filepath.Join(tmpDir, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ssh", "known_hosts"), []byte("corrupt line"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := createHostKeyCallback(); err == nil {
		t.Fatal("createHostKeyCallback() should reject corrupt known_hosts")
	}
}

func TestAppendToKnownHostsUsesBracketedNonDefaultPort(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 2222}
	if err := appendToKnownHosts(path, "example.com", remote, publicKey); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "[example.com]:2222 ") {
		t.Fatalf("known_hosts line = %q", data)
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

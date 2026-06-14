package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
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

func TestInteractiveSessionExitErrorIsACompletedRemoteSession(t *testing.T) {
	var exitErr *ssh.ExitError
	err := fmt.Errorf("wrapped: %w", &ssh.ExitError{})
	if !errors.As(err, &exitErr) {
		t.Fatal("test setup did not produce an SSH exit error")
	}
	if status, ok := remoteSessionExitStatus(err); !ok || status != 0 {
		t.Fatal("remote shell exit should be treated as a completed session")
	}
	if _, ok := remoteSessionExitStatus(errors.New("connection lost")); ok {
		t.Fatal("unexpected disconnect should remain an error")
	}
}

func TestAcceptNewHostKeyIsStoredInConfig(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	if err := config.NewRepositoryWithPath(path).Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	host := config.Host{
		Alias: "server", Host: "example.com", Port: 2222, ConfigPath: path,
		ResolvedHostKeyPolicy: config.HostKeyPolicyAcceptNew,
	}
	cb, err := createHostKeyCallback(host)
	if err != nil {
		t.Fatal(err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 2222}
	if err := cb("example.com:2222", remote, publicKey); err != nil {
		t.Fatal(err)
	}
	doc, err := config.NewRepositoryWithPath(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.HostTrust.Entries) != 1 || doc.HostTrust.Entries[0].Port != 2222 {
		t.Fatalf("host trust entries = %+v", doc.HostTrust.Entries)
	}
}

func TestExplicitInsecureHostPolicyDoesNotRequireStoreLoad(t *testing.T) {
	host := config.Host{
		Alias: "server", Host: "example.com", Port: 22,
		HostKeyPolicy: config.HostKeyPolicyInsecure,
	}
	callback, err := createHostKeyCallback(host)
	if err != nil {
		t.Fatal(err)
	}
	if callback == nil {
		t.Fatal("callback is nil")
	}
}

func TestHostKeyChangeIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	if err := config.NewRepositoryWithPath(path).Replace(config.DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	host := config.Host{
		Alias: "server", Host: "example.com", Port: 22, ConfigPath: path,
		ResolvedHostKeyPolicy: config.HostKeyPolicyAcceptNew,
	}
	cb, err := createHostKeyCallback(host)
	if err != nil {
		t.Fatal(err)
	}
	remote := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	for i := 0; i < 2; i++ {
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		publicKey, err := ssh.NewPublicKey(privateKey.Public())
		if err != nil {
			t.Fatal(err)
		}
		err = cb("example.com:22", remote, publicKey)
		if i == 0 && err != nil {
			t.Fatal(err)
		}
		if i == 1 && err == nil {
			t.Fatal("changed host key should be rejected")
		}
	}
}

func TestStrictUnknownHostIsRejectedWithoutInteractiveTerminal(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	host := config.Host{
		Alias: "server", Host: "example.com", Port: 22,
		ConfigPath:            filepath.Join(t.TempDir(), "sshm.yaml"),
		ResolvedHostKeyPolicy: config.HostKeyPolicyStrict,
	}
	callback, err := createHostKeyCallback(host)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.com:22", &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}, publicKey); err == nil {
		t.Fatal("strict policy should reject an unknown host outside an interactive terminal")
	}
}

func TestAuthStrategyConsts(t *testing.T) {
	seen := map[AuthStrategy]bool{}
	for _, s := range []AuthStrategy{AuthAuto, AuthKey, AuthPassword} {
		if seen[s] {
			t.Errorf("Duplicate AuthStrategy value: %v", s)
		}
		seen[s] = true
	}
}

func TestManagedKeyMaterialLoadsSigner(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "managed")
	if err != nil {
		t.Fatal(err)
	}
	store := secret.NewFileStore(filepath.Join(t.TempDir(), "sshm.yaml"), "master")
	if err := store.SetManagedKey("personal", pem.EncodeToMemory(block)); err != nil {
		t.Fatal(err)
	}
	host := config.Host{Alias: "server", Identity: config.ManagedIdentity("personal")}
	loaded, signer, err := managedKeyMaterial(host, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) == 0 || signer == nil {
		t.Fatal("managed key material was not loaded")
	}
}

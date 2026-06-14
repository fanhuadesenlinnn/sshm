package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryCreatesCommentedSingleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	repo := NewRepositoryWithPath(path)
	doc, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if doc.Defaults.HostKeyPolicy != HostKeyPolicyStrict {
		t.Fatalf("default policy = %q", doc.Defaults.HostKeyPolicy)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"# sshm 配置", "insecure(跳过验证)", "auth: auto | key | password", "tags:", "hosts: []", "managed_keys:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("default config missing %q:\n%s", want, text)
		}
	}
}

func TestRepositoryRejectsDuplicateStableIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	repo := NewRepositoryWithPath(path)
	err := repo.Update(func(doc *Document) error {
		doc.Hosts = []Host{
			{ID: "same", Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto"},
			{ID: "same", Alias: "two", User: "root", Host: "two", Port: 22, Auth: "auto"},
		}
		return nil
	})
	if err == nil {
		t.Fatal("duplicate stable IDs should be rejected")
	}
}

func TestEffectiveHostKeyPolicy(t *testing.T) {
	defaults := Defaults{HostKeyPolicy: HostKeyPolicyAcceptNew}
	if got := (Host{}).EffectiveHostKeyPolicy(defaults); got != HostKeyPolicyAcceptNew {
		t.Fatalf("inherited policy = %q", got)
	}
	if got := (Host{HostKeyPolicy: HostKeyPolicyInsecure}).EffectiveHostKeyPolicy(defaults); got != HostKeyPolicyInsecure {
		t.Fatalf("override policy = %q", got)
	}
}

func TestRepositoryMutationFailureLeavesDocumentUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	repo := NewRepositoryWithPath(path)
	if err := repo.Update(func(doc *Document) error {
		doc.Hosts = append(doc.Hosts, Host{ID: NewID(), Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(func(doc *Document) error {
		doc.Hosts = nil
		return errors.New("injected failure")
	}); err == nil {
		t.Fatal("mutation failure should be returned")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed transaction changed sshm.yaml")
	}
}

func TestRepositoryPersistsGeneratedIDFromHandEditedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	data := []byte(`version: 1
defaults:
  host_key_policy: strict
hosts:
  - alias: manual
    user: root
    host: example.com
managed_keys:
  items: []
host_trust:
  entries: []
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	repo := NewRepositoryWithPath(path)
	first, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if first.Hosts[0].ID == "" || second.Hosts[0].ID != first.Hosts[0].ID {
		t.Fatalf("generated ID was not persisted: %q then %q", first.Hosts[0].ID, second.Hosts[0].ID)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), "id: "+first.Hosts[0].ID) {
		t.Fatalf("generated ID missing from config:\n%s", persisted)
	}
}

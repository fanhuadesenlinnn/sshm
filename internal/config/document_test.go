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
	if err := repo.Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
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
	for _, want := range []string{
		"# sshm 配置文件",
		"# 快速开始：",
		"# 主机密钥策略：",
		"# 主机列表；推荐使用 sshm add",
		"# 加密凭据数据；由 sshm 自动维护",
		"version: 2",
		"tags:",
		"hosts: []",
		"managed_keys:",
		"vault: null",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("default config missing %q:\n%s", want, text)
		}
	}
}

func TestRepositoryRejectsDuplicateStableIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	repo := NewRepositoryWithPath(path)
	if err := repo.Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
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
	if err := repo.Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
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

func TestRepositoryGeneratesAndPersistsMissingHostID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	data := []byte(`version: 2
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
	doc, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Hosts) != 1 || doc.Hosts[0].ID == "" {
		t.Fatalf("missing stable ID was not generated: %+v", doc.Hosts)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), "id: "+doc.Hosts[0].ID) {
		t.Fatalf("generated stable ID was not persisted:\n%s", persisted)
	}
	reloaded, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Hosts[0].ID != doc.Hosts[0].ID {
		t.Fatalf("generated ID changed after reload: %q -> %q", doc.Hosts[0].ID, reloaded.Hosts[0].ID)
	}
}

func TestValidateDocumentDataGeneratesMissingHostID(t *testing.T) {
	doc, err := ValidateDocumentData([]byte(`version: 2
hosts:
  - alias: manual
    user: root
    host: example.com
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Hosts) != 1 || doc.Hosts[0].ID == "" {
		t.Fatalf("missing stable ID was not generated: %+v", doc.Hosts)
	}
}

func TestRepositoryDoesNotRewriteInvalidHostWhenIDIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	data := []byte(`version: 2
hosts:
  - alias: ../invalid
    user: root
    host: example.com
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepositoryWithPath(path).Load(); err == nil {
		t.Fatal("invalid hand-edited host should be rejected")
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(persisted) != string(data) {
		t.Fatalf("invalid hand-edited config was rewritten:\n%s", persisted)
	}
}

func TestConfigV2StrictlyRejectsMissingVersionUnknownFieldsAndUnsafeAlias(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing version", "hosts: []\n", "version"},
		{"unknown top field", "version: 2\nunknown: true\n", "field unknown"},
		{"unknown custom field", "version: 2\ndefaults:\n  logs:\n    unknown: true\n", "未知字段"},
		{"unsafe alias", `version: 2
hosts:
  - id: host-id
    alias: ../escape
    user: root
    host: example.test
`, "别名"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ValidateDocumentData([]byte(tt.body)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryRegistersReferencedTags(t *testing.T) {
	repo := NewRepositoryWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	if err := repo.Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(func(doc *Document) error {
		doc.Tags.Items = append(doc.Tags.Items, Tag{Name: "prod", Note: "生产环境"})
		doc.Hosts = append(doc.Hosts, Host{
			ID: NewID(), Alias: "web", User: "root", Host: "web", Port: 22, Auth: "auto",
			Tags: []string{"linux", "prod", "linux"},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	doc, err := repo.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Hosts[0].Tags, []string{"linux", "prod"}) {
		t.Fatalf("host tags = %v", doc.Hosts[0].Tags)
	}
	if len(doc.Tags.Items) != 2 || doc.Tags.Items[0].Name != "linux" || doc.Tags.Items[1].Name != "prod" {
		t.Fatalf("tag definitions = %+v", doc.Tags.Items)
	}
	if doc.Tags.Items[1].Note != "生产环境" {
		t.Fatalf("tag note was lost: %+v", doc.Tags.Items[1])
	}
}

func TestValidateDocumentRejectsDuplicateTagDefinitions(t *testing.T) {
	_, err := ValidateDocumentData([]byte(`
version: 2
defaults:
  host_key_policy: strict
tags:
  items:
    - name: prod
    - name: prod
hosts: []
managed_keys:
  items: []
host_trust:
  entries: []
`))
	if err == nil || !strings.Contains(err.Error(), "标签名称重复") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTagName(t *testing.T) {
	for _, valid := range []string{"prod", "prod-web", "生产"} {
		if err := ValidateTagName(valid); err != nil {
			t.Fatalf("ValidateTagName(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "all", "-prod", "--all", "prod web", "prod,web", "prod\nweb"} {
		if err := ValidateTagName(invalid); err == nil {
			t.Fatalf("ValidateTagName(%q) should fail", invalid)
		}
	}
}

func TestNormalizeTagsPreservesFirstOccurrenceOrder(t *testing.T) {
	got := NormalizeTags([]string{" web ", "prod", "web", ""})
	want := []string{"web", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeTags() = %v, want %v", got, want)
	}
}

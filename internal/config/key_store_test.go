package config

import (
	"path/filepath"
	"testing"
)

func TestKeyStoreLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	if err := NewRepositoryWithPath(path).Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	store := NewKeyStoreWithPath(path)
	if err := store.Add("personal", "ssh-ed25519 AAAA personal", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Add("work", "ssh-ed25519 BBBB work", false); err != nil {
		t.Fatal(err)
	}
	kf, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if kf.Default != "personal" {
		t.Fatalf("Default = %q, want personal", kf.Default)
	}
	if err := store.SetDefault("work"); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove("personal"); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove("work"); err == nil {
		t.Fatal("Remove(default) should fail")
	}
}

func TestManagedIdentityRoundTrip(t *testing.T) {
	identity := ManagedIdentity("personal")
	name, ok := ManagedKeyName(identity)
	if !ok || name != "personal" {
		t.Fatalf("ManagedKeyName(%q) = %q, %v", identity, name, ok)
	}
	if _, ok := ManagedKeyName("~/.ssh/id_ed25519"); ok {
		t.Fatal("external identity should not be managed")
	}
}

func TestDefaultIsReservedManagedKeyName(t *testing.T) {
	if err := ValidateManagedKeyName("default"); err == nil {
		t.Fatal("default should be reserved")
	}
}

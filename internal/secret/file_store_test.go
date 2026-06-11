package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFileStorePersistsAndVerifiesPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")

	if err := store.SetPassword("server", " ssh:password "); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	if err := store.VerifyPassphrase(); err != nil {
		t.Fatalf("VerifyPassphrase() error = %v", err)
	}

	reopened := NewFileStore(path, "master-password")
	password, err := reopened.GetPassword("server")
	if err != nil {
		t.Fatalf("GetPassword() error = %v", err)
	}
	if password != " ssh:password " {
		t.Fatalf("GetPassword() = %q, want %q", password, " ssh:password ")
	}
}

func TestFileStoreWrongPassphraseDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "correct-password")
	if err := store.SetPassword("server", "original-password"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	wrongStore := NewFileStore(path, "wrong-password")
	err = wrongStore.SetPassword("other", "new-password")
	if !errors.Is(err, ErrIncorrectPassphrase) {
		t.Fatalf("SetPassword() error = %v, want ErrIncorrectPassphrase", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("SetPassword() with a wrong passphrase overwrote secrets.yaml")
	}

	password, err := store.GetPassword("server")
	if err != nil {
		t.Fatalf("GetPassword() error = %v", err)
	}
	if password != "original-password" {
		t.Fatalf("GetPassword() = %q, want %q", password, "original-password")
	}
}

func TestFileStoreCorruptFileDoesNotGetOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	original := []byte("not a valid secrets file")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	store := NewFileStore(path, "master-password")
	if err := store.SetPassword("server", "new-password"); err == nil {
		t.Fatal("SetPassword() error = nil, want corrupt file error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(after) != string(original) {
		t.Fatal("SetPassword() overwrote a corrupt secrets.yaml")
	}
}

func TestFileStoreSetPasswordByID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")

	// Save with SetPasswordByID — should use ID and clear alias
	if err := store.SetPasswordByID("abc123", "old-alias", "secret123"); err != nil {
		t.Fatalf("SetPasswordByID() error = %v", err)
	}

	// Verify can get by ID
	pass, err := store.GetPassword("abc123")
	if err != nil {
		t.Fatalf("GetPassword(abc123) error = %v", err)
	}
	if pass != "secret123" {
		t.Fatalf("GetPassword(abc123) = %q, want secret123", pass)
	}

	// Verify old alias is gone
	_, err = store.GetPassword("old-alias")
	if err == nil {
		t.Fatal("GetPassword(old-alias) should fail after SetPasswordByID")
	}
}

func TestFileStoreSetPasswordByIDRemovesEmptyAliasPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")
	if err := store.SetPassword("old-alias", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPasswordByID("stable-id", "old-alias", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPassword("old-alias"); err == nil {
		t.Fatal("empty alias password was not removed")
	}
}

func TestFileStoreMigrateAliasToID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")

	// First save via old alias
	if err := store.SetPassword("old-alias", "migrate-me"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}

	// Migrate
	migrated, err := store.MigrateAliasToID("old-alias", "new-id-123")
	if err != nil {
		t.Fatalf("MigrateAliasToID() error = %v", err)
	}
	if !migrated {
		t.Fatal("MigrateAliasToID() returned false, want true")
	}

	// Verify by new ID
	pass, err := store.GetPassword("new-id-123")
	if err != nil {
		t.Fatalf("GetPassword(new-id-123) error = %v", err)
	}
	if pass != "migrate-me" {
		t.Fatalf("GetPassword() = %q, want migrate-me", pass)
	}

	// Verify old alias is gone
	_, err = store.GetPassword("old-alias")
	if err == nil {
		t.Fatal("GetPassword(old-alias) should fail after migration")
	}
}

func TestFileStoreMigrateNonExistentAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")

	migrated, err := store.MigrateAliasToID("nonexistent", "some-id")
	if err != nil {
		t.Fatalf("MigrateAliasToID() error = %v", err)
	}
	if migrated {
		t.Fatal("MigrateAliasToID() returned true for nonexistent alias")
	}
}

func TestFileStoreCopyPasswordsKeepsOldReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")
	if err := store.SetPassword("old", "secret"); err != nil {
		t.Fatal(err)
	}
	if err := store.CopyPasswords(map[string]string{"new": "old"}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"old", "new"} {
		if got, err := store.GetPassword(ref); err != nil || got != "secret" {
			t.Fatalf("GetPassword(%q) = %q, %v", ref, got, err)
		}
	}
}

func TestFileStoreBackupBehavior(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewFileStore(path, "master-password")

	if err := store.SetPassword("server", "pass1"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}

	// Read raw file
	data1, _ := os.ReadFile(path)
	if len(data1) == 0 {
		t.Fatal("Secrets file is empty after first write")
	}

	// Write again
	if err := store.SetPassword("server2", "pass2"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}

	data2, _ := os.ReadFile(path)
	if len(data2) == 0 {
		t.Fatal("Secrets file is empty after second write")
	}

	// Both passwords should be readable
	for _, name := range []string{"server", "server2"} {
		if p, err := store.GetPassword(name); err != nil || p == "" {
			t.Fatalf("Lost password for %q after second write", name)
		}
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != string(data1) {
		t.Fatal("backup does not contain previous encrypted file")
	}
}

func TestFileStoreConcurrentWritesDoNotLosePasswords(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "secrets.yaml"), "master-password")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ref := fmt.Sprintf("server-%d", i)
			if err := store.SetPassword(ref, "secret"); err != nil {
				t.Errorf("SetPassword(%s) error = %v", ref, err)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < 10; i++ {
		ref := fmt.Sprintf("server-%d", i)
		if got, err := store.GetPassword(ref); err != nil || got != "secret" {
			t.Fatalf("GetPassword(%s) = %q, %v", ref, got, err)
		}
	}
}

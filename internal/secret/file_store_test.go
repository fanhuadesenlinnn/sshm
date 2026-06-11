package secret

import (
	"errors"
	"os"
	"path/filepath"
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

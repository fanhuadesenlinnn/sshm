package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

func TestFileStorePersistsAndVerifiesPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
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

func TestFileStorePreservesArbitraryPasswordCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := NewFileStore(path, "master-password")
	want := "line one\nline two:with:colons\x00end"
	if err := store.SetPassword("server", want); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetPassword("server")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("password = %q, want %q", got, want)
	}
}

func TestFileStoreWrongPassphraseDoesNotOverwriteExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
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
		t.Fatal("SetPassword() with a wrong passphrase overwrote sshm.yaml")
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
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	original := []byte("not a valid sshm config")
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
		t.Fatal("SetPassword() overwrote a corrupt sshm.yaml")
	}
}

func TestFileStoreDoesNotCreateBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
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

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file: %v", err)
	}
}

func TestFileStoreConcurrentWritesDoNotLosePasswords(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "sshm.yaml"), "master-password")
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

func TestFileStoreManagedKeyAndPasswordCoexist(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "sshm.yaml"), "master-password")
	privateKey := []byte("private:key:data")
	if err := store.SetPassword("server", "password:with:colons"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetManagedKey("personal", privateKey); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetPassword("server"); err != nil || got != "password:with:colons" {
		t.Fatalf("GetPassword() = %q, %v", got, err)
	}
	got, err := store.GetManagedKey("personal")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(privateKey) {
		t.Fatalf("GetManagedKey() = %q, want %q", got, privateKey)
	}
	if err := store.RemoveManagedKeys("personal"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetManagedKey("personal"); err == nil {
		t.Fatal("managed key was not removed")
	}
	if got, err := store.GetPassword("server"); err != nil || got != "password:with:colons" {
		t.Fatalf("password was affected by key removal: %q, %v", got, err)
	}
}

func TestUpdateDocumentFailureLeavesHostAndVaultUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshm.yaml")
	store := NewFileStore(path, "master-password")
	if err := store.SetPassword("stable-id", "original"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		doc.Hosts = append(doc.Hosts, config.Host{ID: config.NewID(), Alias: "new", User: "root", Host: "new", Port: 22, Auth: "auto"})
		entries["password:stable-id"] = "changed"
		return errors.New("injected failure")
	})
	if err == nil {
		t.Fatal("mutation failure should be returned")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed transaction changed sshm.yaml")
	}
	if password, err := store.GetPassword("stable-id"); err != nil || password != "original" {
		t.Fatalf("password = %q, err = %v", password, err)
	}
}

func TestCopiedSingleConfigRetainsHostsAndCredentials(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "sshm.yaml")
	host := config.DefaultHost()
	host.Alias, host.User, host.Host, host.PasswordRef = "portable", "root", "example.test", host.ID
	store := NewFileStore(sourcePath, "master-password")
	if err := store.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		doc.Hosts = []config.Host{host}
		entries["password:"+host.ID] = "secret"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(t.TempDir(), "sshm.yaml")
	if err := os.WriteFile(destinationPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := config.NewStoreWithPath(destinationPath).FindHost(host.Alias)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != host.ID {
		t.Fatalf("copied host ID = %q, want %q", loaded.ID, host.ID)
	}
	password, err := NewFileStore(destinationPath, "master-password").GetPassword(host.ID)
	if err != nil || password != "secret" {
		t.Fatalf("copied password = %q, err = %v", password, err)
	}
}

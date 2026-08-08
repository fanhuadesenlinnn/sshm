package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreLoadEmpty(t *testing.T) {
	store := NewStoreWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	if _, err := store.Load(); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Load() error = %v, want ErrNotInitialized", err)
	}
}

func TestRepositoryUpdateDoesNotCreateMissingConfig(t *testing.T) {
	repo := NewRepositoryWithPath(filepath.Join(t.TempDir(), "sshmd.yaml"))
	err := repo.Update(func(doc *Document) error {
		doc.Hosts = append(doc.Hosts, DefaultHost())
		return nil
	})
	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("Update() error = %v, want ErrNotInitialized", err)
	}
}

func TestStoreSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := newInitializedStore(t, path)

	h := DefaultHost()
	h.Alias = "test-server"
	h.User = "root"
	h.Host = "192.168.1.1"
	h.Port = 22
	h.Note = "test note"
	h.Tags = []string{"web", "db"}

	if h.ID == "" {
		t.Fatal("DefaultHost() returned empty ID")
	}
	if err := store.Add(h); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Reload
	store2 := NewStoreWithPath(path)
	hf, err := store2.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(hf.Hosts) != 1 {
		t.Fatalf("len(Hosts) = %d, want 1", len(hf.Hosts))
	}
	loaded := hf.Hosts[0]
	if loaded.Alias != h.Alias {
		t.Fatalf("Alias = %q, want %q", loaded.Alias, h.Alias)
	}
	if loaded.ID != h.ID {
		t.Fatalf("ID changed: %q -> %q", h.ID, loaded.ID)
	}
	if loaded.Tags == nil || len(loaded.Tags) != 2 {
		t.Fatalf("Tags = %v, want 2 items", loaded.Tags)
	}
}

func TestStoreDuplicateAliasRejected(t *testing.T) {
	store := newInitializedStore(t, filepath.Join(t.TempDir(), "sshmd.yaml"))

	h1 := DefaultHost()
	h1.Alias = "server"
	h1.User = "root"
	h1.Host = "10.0.0.1"
	if err := store.Add(h1); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	h2 := DefaultHost()
	h2.Alias = "server"
	h2.User = "admin"
	h2.Host = "10.0.0.2"
	if err := store.Add(h2); err == nil {
		t.Fatal("Add() with duplicate alias should fail")
	}
}

func TestRepositoryRejectsUnsupportedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	if err := os.WriteFile(path, []byte("version: 99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepositoryWithPath(path).Load(); err == nil {
		t.Fatal("Load() should reject unsupported versions")
	}
}

func TestStoreDuplicateAliasesDetected(t *testing.T) {
	hf := &HostsFile{
		Version: 2,
		Hosts: []Host{
			{Alias: "a", User: "u", Host: "h1"},
			{Alias: "b", User: "u", Host: "h2"},
			{Alias: "a", User: "u", Host: "h3"},
		},
	}
	dups := hf.DuplicateAliases()
	if len(dups) != 1 || dups[0] != "a" {
		t.Fatalf("DuplicateAliases() = %v, want [a]", dups)
	}
}

func TestStoreEnsuresDirsOnSave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-dir", "sub")
	store := newInitializedStore(t, filepath.Join(dir, "sshmd.yaml"))

	h := DefaultHost()
	h.Alias = "test"
	h.User = "root"
	h.Host = "10.0.0.1"
	if err := store.Add(h); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(store.Path()); os.IsNotExist(err) {
		t.Fatal("sshmd.yaml was not created")
	}
}

func TestStoreUpdateChecksDuplicate(t *testing.T) {
	store := newInitializedStore(t, filepath.Join(t.TempDir(), "sshmd.yaml"))

	h1 := DefaultHost()
	h1.Alias = "server1"
	h1.User = "root"
	h1.Host = "10.0.0.1"
	if err := store.Add(h1); err != nil {
		t.Fatal(err)
	}

	h2 := DefaultHost()
	h2.Alias = "server2"
	h2.User = "root"
	h2.Host = "10.0.0.2"
	if err := store.Add(h2); err != nil {
		t.Fatal(err)
	}

	// Try to rename server2 to server1 (duplicate)
	hf, _ := store.Load()
	hf.Hosts[1].Alias = "server1"
	if err := store.Update(1, hf.Hosts[1]); err == nil {
		t.Fatal("Update() with duplicate alias should fail")
	}
}

func TestStoreAtomicWriteSurvives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := newInitializedStore(t, path)

	h := DefaultHost()
	h.Alias = "atomic-test"
	h.User = "root"
	h.Host = "10.0.0.1"
	if err := store.Add(h); err != nil {
		t.Fatal(err)
	}

	// Read back
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Fatal("Atomic write produced empty file")
	}

	// No temp files left behind
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".yaml" && e.Name() != "sshmd.yaml" {
			t.Fatalf("Temp file left behind: %s", e.Name())
		}
	}
}

func TestStoreSaveDoesNotCreateBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	store := newInitializedStore(t, path)
	first := &HostsFile{Hosts: []Host{{ID: NewID(), Alias: "one", User: "root", Host: "one", Port: 22, Auth: "auto"}}}
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	second := &HostsFile{Hosts: []Host{{ID: NewID(), Alias: "two", User: "root", Host: "two", Port: 22, Auth: "auto"}}}
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file: %v", err)
	}
}

func TestStoreCorruptConfigIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshmd.yaml")
	original := []byte("hosts: [not valid")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithPath(path)
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() should reject corrupt config")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("Load() overwrote corrupt config")
	}
}

func TestStoreConcurrentAddsDoNotLoseUpdates(t *testing.T) {
	store := newInitializedStore(t, filepath.Join(t.TempDir(), "sshmd.yaml"))
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h := DefaultHost()
			h.Alias = fmt.Sprintf("server-%d", i)
			h.User = "root"
			h.Host = "example.com"
			if err := store.Add(h); err != nil {
				t.Errorf("Add(%d) error = %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	hf, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(hf.Hosts) != 12 {
		t.Fatalf("len(Hosts) = %d, want 12", len(hf.Hosts))
	}
}

func TestHostValidate(t *testing.T) {
	tests := []struct {
		name    string
		host    Host
		wantErr bool
	}{
		{"valid", Host{Alias: "srv", User: "root", Host: "10.0.0.1", Port: 22, Auth: "auto"}, false},
		{"no alias", Host{User: "root", Host: "10.0.0.1", Port: 22, Auth: "auto"}, true},
		{"numeric alias", Host{Alias: "123", User: "root", Host: "10.0.0.1", Port: 22, Auth: "auto"}, true},
		{"no user", Host{Alias: "srv", Host: "10.0.0.1", Port: 22, Auth: "auto"}, true},
		{"no host", Host{Alias: "srv", User: "root", Port: 22, Auth: "auto"}, true},
		{"bad port", Host{Alias: "srv", User: "root", Host: "10.0.0.1", Port: 0, Auth: "auto"}, true},
		{"bad port max", Host{Alias: "srv", User: "root", Host: "10.0.0.1", Port: 99999, Auth: "auto"}, true},
		{"bad auth", Host{Alias: "srv", User: "root", Host: "10.0.0.1", Port: 22, Auth: "invalid"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.host.Validate()
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr = %v", errs, tt.wantErr)
			}
		})
	}
}

func TestNewIDUniqueness(t *testing.T) {
	ids := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if ids[id] {
			t.Fatalf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestEnsureIDs(t *testing.T) {
	hf := HostsFile{
		Version: 2,
		Hosts: []Host{
			{Alias: "a", User: "u", Host: "h"},
			{Alias: "b", User: "u", Host: "h", ID: "existing"},
		},
	}
	changed := hf.EnsureIDs()
	if !changed {
		t.Fatal("EnsureIDs() should report change")
	}
	if hf.Hosts[0].ID == "" {
		t.Fatal("ID not filled for first host")
	}
	if hf.Hosts[1].ID != "existing" {
		t.Fatal("Existing ID was overwritten")
	}

	// Second call should not change
	changed = hf.EnsureIDs()
	if changed {
		t.Fatal("Second EnsureIDs() should not report change")
	}
}

func TestStoreMarkUsed(t *testing.T) {
	store := newInitializedStore(t, filepath.Join(t.TempDir(), "sshmd.yaml"))
	h := DefaultHost()
	h.Alias = "recent"
	h.User = "root"
	h.Host = "example.com"
	if err := store.Add(h); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkUsed(h.ID, "2026-06-13T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := store.FindHost(h.Alias)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastUsedAt != "2026-06-13T12:00:00Z" {
		t.Fatalf("LastUsedAt = %q", loaded.LastUsedAt)
	}
}

func newInitializedStore(t *testing.T, path string) *Store {
	t.Helper()
	store := NewStoreWithPath(path)
	if err := store.Repository().Replace(DefaultDocument()); err != nil {
		t.Fatal(err)
	}
	return store
}

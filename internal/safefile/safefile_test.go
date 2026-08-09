package safefile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWriteDoesNotCreateBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.yaml")
	if err := Write(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup file: %v", err)
	}
}

func TestWithLockSerializesTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	var wg sync.WaitGroup
	var mu sync.Mutex
	active := 0
	maxActive := 0

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := WithLock(path, func() error {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				active--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Errorf("WithLock() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Fatalf("max concurrent transactions = %d, want 1", maxActive)
	}
}

func TestWithLockDoesNotStealLongRunningLiveLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 2)
	active := 0
	maxActive := 0
	var mu sync.Mutex

	go func() {
		done <- WithLock(path, func() error {
			mu.Lock()
			active++
			maxActive = active
			mu.Unlock()
			close(entered)
			<-release
			mu.Lock()
			active--
			mu.Unlock()
			return nil
		})
	}()
	<-entered
	go func() {
		done <- WithLock(path, func() error {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			active--
			mu.Unlock()
			return nil
		})
	}()
	time.Sleep(100 * time.Millisecond)
	close(release)

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("WithLock() error = %v", err)
		}
	}
	if maxActive != 1 {
		t.Fatalf("max concurrent transactions = %d, want 1", maxActive)
	}
}

func TestWithLockReusesUnlockedSidecar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data")
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte("legacy stale lock\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WithLock(path, func() error { return nil }); err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("advisory lock sidecar should remain stable: %v", err)
	}
}

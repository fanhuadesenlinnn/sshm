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

func TestWithLockHeartbeatPreventsLiveLockFromGoingStale(t *testing.T) {
	oldTimeout, oldStaleAfter, oldHeartbeat := lockTimeout, staleAfter, heartbeatEvery
	lockTimeout = 500 * time.Millisecond
	staleAfter = 60 * time.Millisecond
	heartbeatEvery = 10 * time.Millisecond
	defer func() {
		lockTimeout, staleAfter, heartbeatEvery = oldTimeout, oldStaleAfter, oldHeartbeat
	}()

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
	time.Sleep(staleAfter + 20*time.Millisecond)
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
	time.Sleep(staleAfter + 20*time.Millisecond)
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

func TestWithLockRemovesDeadStaleLock(t *testing.T) {
	oldTimeout, oldStaleAfter := lockTimeout, staleAfter
	lockTimeout = 200 * time.Millisecond
	staleAfter = 20 * time.Millisecond
	defer func() {
		lockTimeout, staleAfter = oldTimeout, oldStaleAfter
	}()

	path := filepath.Join(t.TempDir(), "data")
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Second)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := WithLock(path, func() error { return nil }); err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
}

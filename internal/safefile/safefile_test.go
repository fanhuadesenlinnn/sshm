package safefile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
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

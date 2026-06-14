package sshx

import (
	"strings"
	"sync"
	"testing"
)

func TestSynchronizedBufferAcceptsConcurrentOutput(t *testing.T) {
	var buffer synchronizedBuffer
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if _, err := buffer.Write([]byte("x")); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if got := buffer.String(); len(got) != 800 || strings.Trim(got, "x") != "" {
		t.Fatalf("unexpected output length/content: %d", len(got))
	}
}

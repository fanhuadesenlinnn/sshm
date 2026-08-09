package sshx

import (
	"bytes"
	"io"
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

func TestSynchronizedBufferCapsCaptureAndPreservesStream(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), maxCapturedOutputBytes+4096)
	var capture synchronizedBuffer
	var streamed bytes.Buffer
	n, err := io.MultiWriter(&capture, &streamed).Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("MultiWriter.Write = %d, %v", n, err)
	}
	if !bytes.Equal(streamed.Bytes(), payload) {
		t.Fatalf("外部输出流被截断: got=%d want=%d", streamed.Len(), len(payload))
	}
	got := capture.String()
	if len(got) != maxCapturedOutputBytes+len(truncatedOutputMarker) {
		t.Fatalf("捕获长度 = %d", len(got))
	}
	if !strings.HasSuffix(got, truncatedOutputMarker) {
		t.Fatal("截断后的返回值缺少明确标记")
	}
	if strings.Count(got, truncatedOutputMarker) != 1 {
		t.Fatal("截断标记应只出现一次")
	}
}

package command

import (
	"fmt"
	"sync"
)

type boundedCapture struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func newBoundedCapture(limit int) *boundedCapture {
	if limit < 1 {
		limit = 1
	}
	return &boundedCapture{limit: limit}
}

func (b *boundedCapture) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(data)
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.data = append(b.data, data...)
	}
	if written > remaining {
		b.truncated = true
	}
	return written, nil
}

func (b *boundedCapture) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return string(b.data) + fmt.Sprintf("\n[输出已截断，最多保留 %d 字节]\n", b.limit)
	}
	return string(b.data)
}

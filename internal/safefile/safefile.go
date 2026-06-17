package safefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultLockTimeout = 30 * time.Second
	defaultStaleAfter  = 2 * time.Minute
)

var (
	lockTimeout    = defaultLockTimeout
	staleAfter     = defaultStaleAfter
	heartbeatEvery = 30 * time.Second
)

// WithLock serializes a complete read-modify-write transaction across processes.
func WithLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return fmt.Errorf("创建锁目录失败: %w", err)
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(lock, "%d\n", os.Getpid())
			_ = lock.Close()
			stopHeartbeat := startLockHeartbeat(lockPath)
			defer stopHeartbeat()
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("创建文件锁失败: %w", err)
		}

		if lockIsStale(lockPath) {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待文件锁超时: %s", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func startLockHeartbeat(lockPath string) func() {
	interval := heartbeatEvery
	if interval <= 0 || interval >= staleAfter {
		interval = staleAfter / 4
	}
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				_ = os.Chtimes(lockPath, now, now)
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

func lockIsStale(lockPath string) bool {
	info, err := os.Stat(lockPath)
	if err != nil || time.Since(info.ModTime()) <= staleAfter {
		return false
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return true
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return true
	}
	return !processAlive(pid)
}

// Write atomically replaces a file without creating backup copies.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	return writeAtomic(path, data, perm)
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	closeWithError := func(writeErr error) error {
		if closeErr := tmp.Close(); writeErr == nil {
			return closeErr
		}
		return writeErr
	}

	if err := tmp.Chmod(perm); err != nil {
		return closeWithError(fmt.Errorf("设置临时文件权限失败: %w", err))
	}
	if _, err := tmp.Write(data); err != nil {
		return closeWithError(fmt.Errorf("写入临时文件失败: %w", err))
	}
	if err := tmp.Sync(); err != nil {
		return closeWithError(fmt.Errorf("同步临时文件失败: %w", err))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("原子替换文件失败: %w", err)
	}
	_ = os.Chmod(path, perm)

	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

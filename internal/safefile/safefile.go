package safefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	lockTimeout = 30 * time.Second
	staleAfter  = 2 * time.Minute
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
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("创建文件锁失败: %w", err)
		}

		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > staleAfter {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待文件锁超时: %s", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Write atomically writes data and optionally preserves the previous file as .bak.
func Write(path string, data []byte, perm os.FileMode, backup bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	if backup {
		old, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := writeAtomic(path+".bak", old, perm); err != nil {
				return fmt.Errorf("创建备份失败: %w", err)
			}
		case !errors.Is(err, os.ErrNotExist):
			return fmt.Errorf("读取旧文件以创建备份失败: %w", err)
		}
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

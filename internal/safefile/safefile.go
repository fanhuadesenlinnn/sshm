package safefile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultLockTimeout = 30 * time.Second

var lockTimeout = defaultLockTimeout

// WithLock serializes a complete read-modify-write transaction across processes.
func WithLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return fmt.Errorf("创建锁目录失败: %w", err)
	}
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("打开文件锁失败: %w", err)
	}
	defer lock.Close()

	deadline := time.Now().Add(lockTimeout)
	for {
		acquired, err := tryFileLock(lock)
		if err != nil {
			return fmt.Errorf("获取文件锁失败: %w", err)
		}
		if acquired {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待文件锁超时: %s", lockPath)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer unlockFile(lock)
	if err := Restrict(lockPath, 0600); err != nil {
		return fmt.Errorf("设置文件锁权限失败: %w", err)
	}
	if err := lock.Truncate(0); err == nil {
		_, _ = lock.Seek(0, 0)
		_, _ = fmt.Fprintf(lock, "%d\n", os.Getpid())
	}
	return fn()
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

	if err := Restrict(tmpPath, perm); err != nil {
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
	if err := Restrict(path, perm); err != nil {
		return fmt.Errorf("设置目标文件权限失败: %w", err)
	}

	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

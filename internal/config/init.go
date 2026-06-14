package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
)

// Initialize creates the v2 sshm home and configuration. Existing
// configuration is preserved unless force is true.
func Initialize(force bool) (Paths, string, error) {
	paths, err := ResolvePaths()
	if err != nil {
		return Paths{}, "", err
	}
	if err := EnsureDirs(); err != nil {
		return Paths{}, "", err
	}

	backupPath := ""
	data, readErr := os.ReadFile(paths.Config)
	switch {
	case readErr == nil && !force:
		return Paths{}, "", fmt.Errorf("配置文件已存在: %s；使用 --force 明确覆盖", paths.Config)
	case readErr == nil:
		backupPath = filepath.Join(paths.Backups, "sshm.yaml.bak."+time.Now().Format("20060102-150405"))
		if err := safefile.Write(backupPath, data, 0600); err != nil {
			return Paths{}, "", fmt.Errorf("备份原配置失败: %w", err)
		}
	case !errors.Is(readErr, os.ErrNotExist):
		return Paths{}, "", fmt.Errorf("检查配置文件失败: %w", readErr)
	}

	doc := DefaultDocument()
	encoded, err := encodeDocument(doc)
	if err != nil {
		return Paths{}, "", fmt.Errorf("生成默认配置失败: %w", err)
	}
	if err := safefile.Write(paths.Config, encoded, 0600); err != nil {
		return Paths{}, "", fmt.Errorf("写入默认配置失败: %w", err)
	}
	return paths, backupPath, nil
}

// LegacyConfigExists reports whether the v5 configuration path exists. The
// file is never opened or parsed.
func LegacyConfigExists() (string, bool) {
	path := LegacyConfigPath()
	if path == "" {
		return "", false
	}
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/safefile"
)

// Paths contains every persistent path owned by sshmd.
type Paths struct {
	Home      string
	Config    string
	Logs      string
	Deploy    string
	DeployDir string
	Templates string
	Readme    string
	Backups   string
	Temp      string
}

// ResolvePaths resolves the portable sshmd home. SSHMD_HOME is the only
// supported path override.
func ResolvePaths() (Paths, error) {
	home := os.Getenv("SSHMD_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("解析用户主目录失败: %w", err)
		}
		home = filepath.Join(userHome, ".sshmd")
	} else {
		home = ExpandPath(home)
		if !filepath.IsAbs(home) {
			absolute, err := filepath.Abs(home)
			if err != nil {
				return Paths{}, fmt.Errorf("解析 SSHMD_HOME 失败: %w", err)
			}
			home = absolute
		}
	}
	home = filepath.Clean(home)
	if filepath.Dir(home) == home {
		return Paths{}, fmt.Errorf("SSHMD_HOME 不能是文件系统根目录: %s", home)
	}
	return Paths{
		Home:      home,
		Config:    filepath.Join(home, "sshmd.yaml"),
		Logs:      filepath.Join(home, "logs"),
		Deploy:    filepath.Join(home, "deploy.yaml"),
		DeployDir: filepath.Join(home, "deploy.d"),
		Templates: filepath.Join(home, "templates"),
		Readme:    filepath.Join(home, "README.md"),
		Backups:   filepath.Join(home, "backups"),
		Temp:      filepath.Join(home, "tmp"),
	}, nil
}

// SSHMDHome returns the base directory for all sshmd-owned data.
func SSHMDHome() string {
	paths, err := ResolvePaths()
	if err != nil {
		return filepath.Join(".", ".sshmd")
	}
	return paths.Home
}

// ConfigFilePath returns the only persistent configuration file.
func ConfigFilePath() string {
	paths, err := ResolvePaths()
	if err != nil {
		return filepath.Join(SSHMDHome(), "sshmd.yaml")
	}
	return paths.Config
}

func LogsDir() string {
	paths, err := ResolvePaths()
	if err != nil {
		return filepath.Join(SSHMDHome(), "logs")
	}
	return paths.Logs
}

func DeployFilePath() string {
	return filepath.Join(SSHMDHome(), "deploy.yaml")
}

func DeployDir() string {
	return filepath.Join(SSHMDHome(), "deploy.d")
}

func BackupsDir() string {
	return filepath.Join(SSHMDHome(), "backups")
}

func TempDir() string {
	return filepath.Join(SSHMDHome(), "tmp")
}

// LegacyConfigPath returns the old location only for warning users. v6 never
// reads business data from this path.
func LegacyConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sshmd", "sshmd.yaml")
}

// EnsureDirs creates all required directories with proper permissions.
func EnsureDirs() error {
	paths, err := ResolvePaths()
	if err != nil {
		return err
	}
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{paths.Home, 0700},
		{paths.DeployDir, 0700},
		{paths.Templates, 0700},
		{paths.Logs, 0700},
		{paths.Backups, 0700},
		{paths.Temp, 0700},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", d.path, err)
		}
		if err := safefile.Restrict(d.path, d.perm); err != nil {
			return fmt.Errorf("设置目录权限 %s 失败: %w", d.path, err)
		}
	}
	return nil
}

// ExpandPath resolves a path that may be absolute, start with ~, or be relative
// to the current working directory.
func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(p) >= 2 && (p[:2] == "~/" || p[:2] == `~\`) {
		home, _ := os.UserHomeDir()
		rest := strings.ReplaceAll(p[2:], `\`, string(filepath.Separator))
		return filepath.Join(home, rest)
	}
	absolute, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return absolute
}

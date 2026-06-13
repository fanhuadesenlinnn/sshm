package config

import (
	"os"
	"path/filepath"
)

// SSHMHome returns the base configuration directory for sshm.
// Priority: SSHM_HOME > XDG_CONFIG_HOME/sshm > ~/.config/sshm
func SSHMHome() string {
	if v := os.Getenv("SSHM_HOME"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "sshm")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sshm")
}

// ConfigFilePath returns the only persistent configuration file.
func ConfigFilePath() string {
	if v := os.Getenv("SSHM_CONFIG_FILE"); v != "" {
		return v
	}
	return filepath.Join(SSHMHome(), "sshm.yaml")
}

func LogsDir() string {
	return filepath.Join(SSHMHome(), "logs")
}

// EnsureDirs creates all required directories with proper permissions.
func EnsureDirs() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{SSHMHome(), 0700},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return err
		}
		if err := os.Chmod(d.path, d.perm); err != nil {
			return err
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
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	absolute, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return absolute
}

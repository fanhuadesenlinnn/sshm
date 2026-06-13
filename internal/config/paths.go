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

// HostsFilePath returns the path to hosts.yaml.
func HostsFilePath() string {
	if v := os.Getenv("SSHM_CONFIG_FILE"); v != "" {
		return v
	}
	return filepath.Join(SSHMHome(), "hosts.yaml")
}

// SecretsFilePath returns the path to secrets.yaml.
func SecretsFilePath() string {
	if v := os.Getenv("SSHM_SECRET_FILE"); v != "" {
		return v
	}
	return filepath.Join(SSHMHome(), "secrets.yaml")
}

// ManagedKeysFilePath returns the path to managed key metadata.
func ManagedKeysFilePath() string {
	if v := os.Getenv("SSHM_MANAGED_KEYS_FILE"); v != "" {
		return v
	}
	return filepath.Join(SSHMHome(), "keys.yaml")
}

// KeysDir returns the path to the keys directory.
func KeysDir() string {
	if v := os.Getenv("SSHM_KEYS_DIR"); v != "" {
		return v
	}
	return filepath.Join(SSHMHome(), "keys")
}

// SSHConfigPath returns the path to the generated ssh_config.
func SSHConfigPath() string {
	return filepath.Join(SSHMHome(), "ssh_config")
}

// EnsureDirs creates all required directories with proper permissions.
func EnsureDirs() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{SSHMHome(), 0700},
		{KeysDir(), 0700},
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

// ExpandPath resolves a path that may be absolute, start with ~, or be relative.
// Relative paths are resolved against SSHMHome.
func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return filepath.Join(SSHMHome(), p)
}

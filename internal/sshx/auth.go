package sshx

import (
	"fmt"
	"os"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/secret"
)

// Connect orchestrates the SSH connection based on auth strategy.
func Connect(h config.Host, store *secret.FileStore, extraArgs []string) error {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		return connectKey(h, extraArgs)
	case AuthPassword:
		return connectPassword(h, store, extraArgs)
	case AuthAsk:
		return connectSystem(h, extraArgs)
	case AuthSystem:
		return connectSystem(h, extraArgs)
	default:
		return connectAuto(h, store, extraArgs)
	}
}

func connectAuto(h config.Host, store *secret.FileStore, extraArgs []string) error {
	hasKey := HasIdentity(h)
	hasPassword := h.PasswordRef != "" && store != nil

	if hasKey {
		if code := ConnectOpenSSHKey(h, extraArgs); code == 0 {
			return nil
		}
		// Key failed
	}

	if hasPassword && store != nil {
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			return connectSystem(h, extraArgs)
		}
		if err := NativeConnectPassword(h, pass); err != nil {
			return connectSystem(h, extraArgs)
		}
		return nil
	}

	if !hasKey && !hasPassword {
		return connectSystem(h, extraArgs)
	}

	// Fallback to system
	return connectSystem(h, extraArgs)
}

func connectKey(h config.Host, extraArgs []string) error {
	if !HasIdentity(h) {
		return fmt.Errorf("主机 %s 未配置密钥", h.Alias)
	}
	code := ConnectOpenSSHKey(h, extraArgs)
	if code != 0 {
		return fmt.Errorf("密钥认证失败 (exit %d)", code)
	}
	return nil
}

func connectPassword(h config.Host, store *secret.FileStore, extraArgs []string) error {
	if h.PasswordRef == "" || store == nil {
		return fmt.Errorf("主机 %s 未配置密码", h.Alias)
	}
	pass, err := store.GetPassword(h.PasswordRef)
	if err != nil {
		return err
	}
	// For password connections that may need extra args, use system ssh with sshpass if available
	// Otherwise use native
	return NativeConnectPassword(h, pass)
}

func connectSystem(h config.Host, extraArgs []string) error {
	code := ConnectOpenSSHDefault(h, extraArgs)
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// ExecCommand runs a command on a host, using the best available auth.
func ExecCommand(h config.Host, store *secret.FileStore, command string) (string, error) {
	strategy := GetAuthStrategy(h.Auth)

	if strategy == AuthKey || (strategy == AuthAuto && HasIdentity(h)) {
		return ExecOpenSSH(h, command)
	}

	if h.PasswordRef != "" && store != nil {
		pass, err := store.GetPassword(h.PasswordRef)
		if err == nil {
			return NativeExec(h, pass, command)
		}
	}

	return ExecOpenSSH(h, command)
}

// CheckPing pings a host using the best available auth.
func CheckPing(h config.Host, store *secret.FileStore) (bool, string) {
	strategy := GetAuthStrategy(h.Auth)

	if strategy == AuthKey || (strategy == AuthAuto && HasIdentity(h)) {
		return Ping(h)
	}

	if h.PasswordRef != "" && store != nil {
		pass, err := store.GetPassword(h.PasswordRef)
		if err == nil {
			return NativePing(h, pass)
		}
	}

	return Ping(h)
}

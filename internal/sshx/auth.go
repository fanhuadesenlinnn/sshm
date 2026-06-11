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

	// When both key and password are available, try key first then password.
	// Users can force password-first by setting auth: password.
	if hasKey {
		if code := ConnectOpenSSHKey(h, extraArgs); code == 0 {
			return nil
		}
		if hasPassword {
			fmt.Fprintf(os.Stderr, "密钥认证未通过，尝试密码连接...\n")
		}
	}

	// Password auth — treated as a first-class method, not just a fallback.
	if hasPassword {
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取密码失败: %v，回退到系统 SSH\n", err)
			return connectSystem(h, extraArgs)
		}
		if err := NativeConnectPassword(h, pass); err != nil {
			fmt.Fprintf(os.Stderr, "密码连接失败: %v，回退到系统 SSH\n", err)
			return connectSystem(h, extraArgs)
		}
		return nil
	}

	if !hasKey && !hasPassword {
		return connectSystem(h, extraArgs)
	}

	// Key-only case: key failed, no password configured
	fmt.Fprintf(os.Stderr, "密钥认证未通过，回退到系统 SSH...\n")
	return connectSystem(h, extraArgs)
}

func connectKey(h config.Host, extraArgs []string) error {
	if !HasIdentity(h) {
		return fmt.Errorf("主机 %s 未配置密钥", h.Alias)
	}
	code := ConnectOpenSSHKey(h, extraArgs)
	if code != 0 {
		return fmt.Errorf("密钥认证失败 (exit %d)，请检查密钥路径和权限", code)
	}
	return nil
}

func connectPassword(h config.Host, store *secret.FileStore, extraArgs []string) error {
	if h.PasswordRef == "" || store == nil {
		return fmt.Errorf("主机 %s 未配置密码", h.Alias)
	}
	pass, err := store.GetPassword(h.PasswordRef)
	if err != nil {
		return fmt.Errorf("读取密码失败: %w", err)
	}
	if err := NativeConnectPassword(h, pass); err != nil {
		return fmt.Errorf("密码认证失败: %w", err)
	}
	return nil
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

	switch strategy {
	case AuthKey:
		ok, msg := Ping(h)
		if !ok {
			return false, "[密钥] " + msg
		}
		return ok, msg
	case AuthPassword:
		if h.PasswordRef != "" && store != nil {
			pass, err := store.GetPassword(h.PasswordRef)
			if err != nil {
				return false, "[密码] 读取密码失败: " + err.Error()
			}
			ok, msg := NativePing(h, pass)
			if !ok {
				return false, "[密码] " + msg
			}
			return ok, msg
		}
		return false, "[密码] 未配置密码"
	case AuthSystem, AuthAsk:
		ok, msg := Ping(h)
		if !ok {
			return false, "[系统] " + msg
		}
		return ok, msg
	default: // AuthAuto
		hasKey := HasIdentity(h)
		hasPassword := h.PasswordRef != "" && store != nil

		if hasKey {
			ok, msg := Ping(h)
			if ok {
				return true, msg
			}
			if hasPassword {
				// Key failed, try password
				pass, err := store.GetPassword(h.PasswordRef)
				if err != nil {
					return false, "[密钥] " + msg + " | [密码] 读取失败"
				}
				ok2, msg2 := NativePing(h, pass)
				if !ok2 {
					return false, "[密钥] " + msg + " | [密码] " + msg2
				}
				return ok2, msg2
			}
			return false, "[密钥] " + msg
		}

		if hasPassword {
			pass, err := store.GetPassword(h.PasswordRef)
			if err != nil {
				return false, "[密码] 读取密码失败: " + err.Error()
			}
			ok, msg := NativePing(h, pass)
			if !ok {
				return false, "[密码] " + msg
			}
			return ok, msg
		}

		ok, msg := Ping(h)
		if !ok {
			return false, "[系统] " + msg
		}
		return ok, msg
	}
}

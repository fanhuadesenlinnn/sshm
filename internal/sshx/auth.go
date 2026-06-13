package sshx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"golang.org/x/crypto/ssh"
)

// Connect orchestrates the SSH connection based on auth strategy.
func Connect(h config.Host, store *secret.FileStore, extraArgs []string) error {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		return connectKeyWithStore(h, store, extraArgs)
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
	hasKey := HasIdentity(h) || isManagedIdentity(h)
	hasPasswordRef := h.PasswordRef != "" && store != nil
	hasPassword := hasPasswordRef

	// When both key and password are available, try key first then password.
	if hasKey {
		if isManagedIdentity(h) {
			if err := ConnectManagedKey(h, store, extraArgs); err == nil {
				return nil
			}
		} else {
			if code := ConnectOpenSSHKey(h, extraArgs); code == 0 {
				return nil
			}
		}
		if hasPassword {
			fmt.Fprintf(os.Stderr, "密钥认证未通过，尝试密码连接...\n")
		}
	}

	if hasPassword {
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取密码失败: %v，回退到系统 SSH\n", err)
			return connectSystem(h, extraArgs)
		}
		if len(extraArgs) > 0 {
			fmt.Fprintln(os.Stderr, "密码连接不支持额外 SSH 参数，回退到系统 SSH...")
			return connectSystem(h, extraArgs)
		}
		if err := NativeConnectPassword(h, pass); err != nil {
			fmt.Fprintf(os.Stderr, "密码连接失败: %v，回退到系统 SSH\n", err)
			return connectSystem(h, extraArgs)
		}
		return nil
	}

	if !hasKey && !hasPasswordRef {
		return connectSystem(h, extraArgs)
	}

	// Key-only case: key failed, no password configured
	fmt.Fprintf(os.Stderr, "密钥认证未通过，回退到系统 SSH...\n")
	return connectSystem(h, extraArgs)
}

func connectKey(h config.Host, extraArgs []string) error {
	return connectKeyWithStore(h, nil, extraArgs)
}

func connectKeyWithStore(h config.Host, store *secret.FileStore, extraArgs []string) error {
	if isManagedIdentity(h) {
		return ConnectManagedKey(h, store, extraArgs)
	}
	if !HasIdentity(h) {
		return fmt.Errorf("主机 %s 未配置密钥", h.Alias)
	}
	code := ConnectOpenSSHKey(h, extraArgs)
	if code != 0 {
		return fmt.Errorf("密钥认证失败 (exit %d)，请检查密钥路径和权限", code)
	}
	return nil
}

// ConnectManagedKey connects with a master-password-protected private key.
func ConnectManagedKey(h config.Host, store *secret.FileStore, extraArgs []string) error {
	privateKey, signer, err := managedKeyMaterial(h, store)
	if err != nil {
		return err
	}
	if len(extraArgs) == 0 {
		return NativeConnectAuth(h, ssh.PublicKeys(signer), "托管密钥")
	}
	tempDir, err := os.MkdirTemp("", "sshm-key-*")
	if err != nil {
		return fmt.Errorf("创建临时密钥目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)
	path := filepath.Join(tempDir, "identity")
	if err := os.WriteFile(path, privateKey, 0600); err != nil {
		return fmt.Errorf("写入会话临时密钥失败: %w", err)
	}
	tempHost := h
	tempHost.Identity = path
	if code := ConnectOpenSSHKey(tempHost, extraArgs); code != 0 {
		return fmt.Errorf("托管密钥认证失败 (exit %d)", code)
	}
	return nil
}

func connectPassword(h config.Host, store *secret.FileStore, extraArgs []string) error {
	if len(extraArgs) > 0 {
		return fmt.Errorf("password 认证不支持额外 SSH 参数；请改用 system/ask 认证策略")
	}
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
		return fmt.Errorf("系统 SSH 连接失败 (exit %d)", code)
	}
	return nil
}

// ConnectSystem connects using system SSH (default behavior).
func ConnectSystem(h config.Host, extraArgs []string) error {
	return connectSystem(h, extraArgs)
}

// ExecCommand runs a command on a host, using the best available auth.
func ExecCommand(h config.Host, store *secret.FileStore, command string) (string, error) {
	return ExecCommandContext(context.Background(), h, store, command)
}

// ExecCommandContext runs a command using a consistent authentication plan.
func ExecCommandContext(ctx context.Context, h config.Host, store *secret.FileStore, command string) (string, error) {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		if isManagedIdentity(h) {
			_, signer, err := managedKeyMaterial(h, store)
			if err != nil {
				return "", err
			}
			return NativeExecAuthContext(ctx, h, ssh.PublicKeys(signer), "托管密钥", command)
		}
		if !HasIdentity(h) {
			return "", fmt.Errorf("主机 %s 未配置可用密钥", h.Alias)
		}
		return ExecOpenSSHContext(ctx, h, command)
	case AuthPassword:
		if h.PasswordRef == "" || store == nil {
			return "", fmt.Errorf("主机 %s 未配置或未解锁密码", h.Alias)
		}
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			return "", fmt.Errorf("读取密码失败: %w", err)
		}
		return NativeExecContext(ctx, h, pass, command)
	case AuthAsk, AuthSystem:
		return ExecOpenSSHDefaultContext(ctx, h, command)
	}

	var keyErr error
	var passwordErr error
	if HasIdentity(h) || isManagedIdentity(h) {
		var output string
		var err error
		if isManagedIdentity(h) {
			var signer ssh.Signer
			_, signer, err = managedKeyMaterial(h, store)
			if err == nil {
				output, err = NativeExecAuthContext(ctx, h, ssh.PublicKeys(signer), "托管密钥", command)
			}
		} else {
			output, err = ExecOpenSSHContext(ctx, h, command)
		}
		if err == nil {
			return output, nil
		}
		keyErr = err
	}
	if h.PasswordRef != "" && store != nil {
		pass, err := store.GetPassword(h.PasswordRef)
		if err == nil {
			output, passErr := NativeExecContext(ctx, h, pass, command)
			if passErr == nil {
				return output, nil
			}
			passwordErr = passErr
		} else {
			passwordErr = err
		}
	}
	output, systemErr := ExecOpenSSHDefaultContext(ctx, h, command)
	if systemErr != nil && (keyErr != nil || passwordErr != nil) {
		return output, fmt.Errorf("auto 认证失败（密钥: %v；密码: %v；系统: %w）", keyErr, passwordErr, systemErr)
	}
	return output, systemErr
}

// CheckPing pings a host using the best available auth.
func CheckPing(h config.Host, store *secret.FileStore) (bool, string) {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		if isManagedIdentity(h) {
			_, signer, err := managedKeyMaterial(h, store)
			if err != nil {
				return false, "[托管密钥] " + err.Error()
			}
			return NativePingAuth(h, ssh.PublicKeys(signer), "托管密钥")
		}
		if !HasIdentity(h) {
			return false, "[密钥] 未配置可用密钥"
		}
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
		ok, msg := PingDefault(h)
		if !ok {
			return false, "[系统] " + msg
		}
		return ok, msg
	default: // AuthAuto
		hasKey := HasIdentity(h) || isManagedIdentity(h)
		hasPassword := h.PasswordRef != "" && store != nil

		if hasKey {
			var ok bool
			var msg string
			if isManagedIdentity(h) {
				_, signer, err := managedKeyMaterial(h, store)
				if err != nil {
					ok, msg = false, err.Error()
				} else {
					ok, msg = NativePingAuth(h, ssh.PublicKeys(signer), "托管密钥")
				}
			} else {
				ok, msg = Ping(h)
			}
			if ok {
				return true, msg
			}
			if hasPassword {
				pass, err := store.GetPassword(h.PasswordRef)
				if err != nil {
					ok3, msg3 := PingDefault(h)
					if ok3 {
						return true, msg3
					}
					return false, "[密钥] " + msg + " | [密码] 读取失败 | [系统] " + msg3
				}
				ok2, msg2 := NativePing(h, pass)
				if !ok2 {
					ok3, msg3 := PingDefault(h)
					if ok3 {
						return true, msg3
					}
					return false, "[密钥] " + msg + " | [密码] " + msg2 + " | [系统] " + msg3
				}
				return ok2, msg2
			}
			ok, systemMsg := PingDefault(h)
			if ok {
				return true, systemMsg
			}
			return false, "[密钥] " + msg + " | [系统] " + systemMsg
		}

		if hasPassword {
			pass, err := store.GetPassword(h.PasswordRef)
			if err != nil {
				ok, msg := PingDefault(h)
				if ok {
					return true, msg
				}
				return false, "[密码] 读取密码失败: " + err.Error() + " | [系统] " + msg
			}
			ok, msg := NativePing(h, pass)
			if !ok {
				ok2, msg2 := PingDefault(h)
				if ok2 {
					return true, msg2
				}
				return false, "[密码] " + msg + " | [系统] " + msg2
			}
			return ok, msg
		}

		ok, msg := PingDefault(h)
		if !ok {
			return false, "[系统] " + msg
		}
		return ok, msg
	}
}

func isManagedIdentity(h config.Host) bool {
	_, ok := config.ManagedKeyName(h.Identity)
	return ok
}

func managedKeyMaterial(h config.Host, store *secret.FileStore) ([]byte, ssh.Signer, error) {
	name, ok := config.ManagedKeyName(h.Identity)
	if !ok {
		return nil, nil, fmt.Errorf("主机 %s 未配置托管密钥", h.Alias)
	}
	if store == nil {
		return nil, nil, fmt.Errorf("托管密钥 %s 需要先解锁 sshm 密码库", name)
	}
	privateKey, err := store.GetManagedKey(name)
	if err != nil {
		return nil, nil, err
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("解析托管密钥 %s 失败: %w", name, err)
	}
	return privateKey, signer, nil
}

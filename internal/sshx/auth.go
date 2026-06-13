package sshx

import (
	"context"
	"fmt"
	"os"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"golang.org/x/crypto/ssh"
)

// Connect dials an interactive SSH session using the best available auth.
func Connect(h config.Host, store *secret.FileStore) error {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		return connectManagedKey(h, store)
	case AuthPassword:
		return connectPassword(h, store)
	default: // AuthAuto
		return connectAuto(h, store)
	}
}

func connectAuto(h config.Host, store *secret.FileStore) error {
	hasManagedKey := isManagedIdentity(h)
	hasPassword := h.PasswordRef != "" && store != nil

	// Try managed key first.
	if hasManagedKey {
		if err := connectManagedKey(h, store); err == nil {
			return nil
		}
		if hasPassword {
			fmt.Fprintf(os.Stderr, "托管密钥认证未通过，尝试密码连接...\n")
		}
	}

	// Try saved password.
	if hasPassword {
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			return fmt.Errorf("读取密码失败: %w", err)
		}
		if err := NativeConnectPassword(h, pass); err != nil {
			return fmt.Errorf("密码认证失败: %w", err)
		}
		return nil
	}

	if !hasManagedKey {
		return fmt.Errorf("主机 %s 未配置托管密钥或密码", h.Alias)
	}
	return fmt.Errorf("托管密钥认证未通过，且未配置密码")
}

// connectManagedKey connects with a master-password-protected private key.
func connectManagedKey(h config.Host, store *secret.FileStore) error {
	if !isManagedIdentity(h) {
		return fmt.Errorf("主机 %s 未配置托管密钥", h.Alias)
	}
	_, signer, err := managedKeyMaterial(h, store)
	if err != nil {
		return err
	}
	return NativeConnectAuth(h, ssh.PublicKeys(signer), "托管密钥")
}

// connectPassword connects with a saved password.
func connectPassword(h config.Host, store *secret.FileStore) error {
	if h.PasswordRef == "" || store == nil {
		return fmt.Errorf("主机 %s 未配置或未解锁密码", h.Alias)
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

// ExecCommand runs a command on a host using the best available auth.
func ExecCommand(h config.Host, store *secret.FileStore, command string) (string, error) {
	return ExecCommandContext(context.Background(), h, store, command)
}

// ExecCommandContext runs a command using a consistent authentication plan.
func ExecCommandContext(ctx context.Context, h config.Host, store *secret.FileStore, command string) (string, error) {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		if !isManagedIdentity(h) {
			return "", fmt.Errorf("主机 %s 未配置托管密钥", h.Alias)
		}
		_, signer, err := managedKeyMaterial(h, store)
		if err != nil {
			return "", err
		}
		return NativeExecAuthContext(ctx, h, ssh.PublicKeys(signer), "托管密钥", command)
	case AuthPassword:
		if h.PasswordRef == "" || store == nil {
			return "", fmt.Errorf("主机 %s 未配置或未解锁密码", h.Alias)
		}
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			return "", fmt.Errorf("读取密码失败: %w", err)
		}
		return NativeExecContext(ctx, h, pass, command)
	default: // AuthAuto
		return execAuto(ctx, h, store, command)
	}
}

func execAuto(ctx context.Context, h config.Host, store *secret.FileStore, command string) (string, error) {
	// Try managed key first.
	if isManagedIdentity(h) {
		_, signer, err := managedKeyMaterial(h, store)
		if err == nil {
			output, keyErr := NativeExecAuthContext(ctx, h, ssh.PublicKeys(signer), "托管密钥", command)
			if keyErr == nil {
				return output, nil
			}
		}
	}

	// Try saved password.
	if h.PasswordRef != "" && store != nil {
		pass, err := store.GetPassword(h.PasswordRef)
		if err == nil {
			output, passErr := NativeExecContext(ctx, h, pass, command)
			if passErr == nil {
				return output, nil
			}
			return "", fmt.Errorf("auto 认证失败（密码: %w）", passErr)
		}
	}

	return "", fmt.Errorf("主机 %s 未配置可用的认证凭据", h.Alias)
}

// CheckPing tests connectivity using the best available auth.
func CheckPing(h config.Host, store *secret.FileStore) (bool, string) {
	strategy := GetAuthStrategy(h.Auth)

	switch strategy {
	case AuthKey:
		if !isManagedIdentity(h) {
			return false, "[密钥] 未配置托管密钥"
		}
		_, signer, err := managedKeyMaterial(h, store)
		if err != nil {
			return false, "[托管密钥] " + err.Error()
		}
		return NativePingAuth(h, ssh.PublicKeys(signer), "托管密钥")
	case AuthPassword:
		if h.PasswordRef != "" && store != nil {
			pass, err := store.GetPassword(h.PasswordRef)
			if err != nil {
				return false, "[密码] 读取密码失败: " + err.Error()
			}
			return NativePing(h, pass)
		}
		return false, "[密码] 未配置密码"
	default: // AuthAuto
		return pingAuto(h, store)
	}
}

func pingAuto(h config.Host, store *secret.FileStore) (bool, string) {
	hasManagedKey := isManagedIdentity(h)
	hasPassword := h.PasswordRef != "" && store != nil

	// Try managed key first.
	if hasManagedKey {
		_, signer, err := managedKeyMaterial(h, store)
		if err != nil {
			if !hasPassword {
				return false, "[托管密钥] " + err.Error()
			}
		} else {
			ok, msg := NativePingAuth(h, ssh.PublicKeys(signer), "托管密钥")
			if ok {
				return true, msg
			}
			if !hasPassword {
				return false, "[托管密钥] " + msg
			}
		}
	}

	// Try saved password.
	if hasPassword {
		pass, err := store.GetPassword(h.PasswordRef)
		if err != nil {
			return false, "[密码] 读取密码失败: " + err.Error()
		}
		return NativePing(h, pass)
	}

	return false, "[认证] 未配置可用的认证凭据"
}

// isManagedIdentity reports whether the host uses an sshm-managed key.
func isManagedIdentity(h config.Host) bool {
	_, ok := config.ManagedKeyName(h.Identity)
	return ok
}

// managedKeyMaterial returns the raw private key and a signer for a managed key.
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

// ManagedKeySigner returns a signer for a host's managed key.
func ManagedKeySigner(h config.Host, store *secret.FileStore) (ssh.Signer, error) {
	_, s, err := managedKeyMaterial(h, store)
	return s, err
}

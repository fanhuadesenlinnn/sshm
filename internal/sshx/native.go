package sshx

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var hostTrustPromptMu sync.Mutex

func createHostKeyCallback(h config.Host) (ssh.HostKeyCallback, error) {
	policy := h.ResolvedHostKeyPolicy
	if policy == "" {
		policy = h.HostKeyPolicy
	}
	if policy == "" {
		policy = config.HostKeyPolicyStrict
	}
	if policy == config.HostKeyPolicyInsecure {
		fmt.Fprintf(os.Stderr, "%s\n", ui.Warn("主机 %s 已配置为跳过身份验证", h.Alias))
		return ssh.InsecureIgnoreHostKey(), nil
	}
	if !config.ValidHostKeyPolicy(policy) {
		return nil, fmt.Errorf("无效的主机信任策略: %s", policy)
	}
	path := h.ConfigPath
	if path == "" {
		path = config.ConfigFilePath()
	}
	repo := config.NewRepositoryWithPath(path)
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		hostTrustPromptMu.Lock()
		defer hostTrustPromptMu.Unlock()

		doc, err := repo.Load()
		if err != nil {
			return err
		}
		publicKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		for _, entry := range doc.HostTrust.Entries {
			if entry.Host != h.Host || entry.Port != h.Port {
				continue
			}
			if entry.PublicKey == publicKey {
				return nil
			}
			return fmt.Errorf("主机密钥验证失败: %s (%s:%d) 的密钥已变化", h.Alias, h.Host, h.Port)
		}

		if policy == config.HostKeyPolicyStrict {
			if !ui.IsTerminal() {
				return fmt.Errorf("无法验证主机 %s 的身份；请先在交互终端确认，或配置 accept-new/insecure", h.Alias)
			}
			fmt.Fprintf(os.Stderr, "\n首次连接 %s (%s:%d)\n", h.Alias, h.Host, h.Port)
			fmt.Fprintf(os.Stderr, "  类型: %s\n  SHA256: %s\n\n", key.Type(), ssh.FingerprintSHA256(key))
			if !ui.ReadYesNo("是否信任该主机密钥并继续连接？[y/N]: ") {
				return fmt.Errorf("用户拒绝了主机 %s 的密钥", h.Alias)
			}
		}

		return repo.Update(func(doc *config.Document) error {
			for _, entry := range doc.HostTrust.Entries {
				if entry.Host == h.Host && entry.Port == h.Port {
					if entry.PublicKey == publicKey {
						return nil
					}
					return fmt.Errorf("主机密钥验证失败: %s 的密钥已变化", h.Alias)
				}
			}
			doc.HostTrust.Entries = append(doc.HostTrust.Entries, config.HostTrustEntry{
				Host: h.Host, Port: h.Port, PublicKey: publicKey,
			})
			return nil
		})
	}, nil
}

func interactiveSession(client *ssh.Client, label string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("%s认证 - 创建会话失败: %w", label, err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("设置终端原始模式失败: %w", err)
	}
	defer term.Restore(fd, oldState)

	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		termWidth = 80
		termHeight = 24
	}

	if err := session.RequestPty("xterm-256color", termHeight, termWidth, modes); err != nil {
		return fmt.Errorf("请求 PTY 失败: %w", err)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(sigCh, syscallSignal())
	go func() {
		for {
			select {
			case <-done:
				return
			case <-sigCh:
				if w, h, err := term.GetSize(fd); err == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}
	}()
	defer func() {
		close(done)
		signal.Stop(sigCh)
	}()

	if err := session.Shell(); err != nil {
		return fmt.Errorf("启动 Shell 失败: %w", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("远程会话异常结束: %w", err)
	}
	return nil
}

// AuthStrategy determines how to connect to a host.
type AuthStrategy int

const (
	AuthAuto     AuthStrategy = iota
	AuthKey      AuthStrategy = iota
	AuthPassword AuthStrategy = iota
)

// GetAuthStrategy maps a string to AuthStrategy.
func GetAuthStrategy(auth string) AuthStrategy {
	switch auth {
	case "key":
		return AuthKey
	case "password":
		return AuthPassword
	default:
		return AuthAuto
	}
}

// syscallSignal returns the appropriate signal for terminal resize.
func syscallSignal() os.Signal {
	return syscallSigwinch()
}

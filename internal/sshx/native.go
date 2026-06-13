package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/safefile"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

var hostTrustPromptMu sync.Mutex

// knownHostsPath returns the default known_hosts path.
func knownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "known_hosts")
}

// ensureSSHDir creates ~/.ssh with 0700 if it doesn't exist.
func ensureSSHDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户目录: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("无法创建 ~/.ssh 目录: %w", err)
	}
	return os.Chmod(sshDir, 0700)
}

// createHostKeyCallback builds a strict known-hosts verification callback.
//
// Behaviour:
//   - known & matching → allow
//   - known but mismatched → reject with clear error
//   - first connection, terminal attached → show fingerprint, ask, atomically append
//   - first connection, NO terminal → reject, do NOT auto-accept
func createHostKeyCallback() (ssh.HostKeyCallback, error) {
	khPath := knownHostsPath()
	cb, cbErr := knownhosts.New(khPath)

	if cbErr == nil {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			err := cb(hostname, remote, key)
			if err == nil {
				return nil
			}
			return classifyAndHandleHostKeyError(khPath, hostname, remote, key, err)
		}, nil
	}

	if _, statErr := os.Stat(khPath); statErr == nil {
		return nil, fmt.Errorf("无法读取 known_hosts %s: %w", khPath, cbErr)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("无法访问 known_hosts %s: %w", khPath, statErr)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return handleUnknownHost(khPath, hostname, remote, key)
	}, nil
}

// classifyAndHandleHostKeyError decides what to do with a host key error.
func classifyAndHandleHostKeyError(khPath, hostname string, remote net.Addr, key ssh.PublicKey, err error) error {
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
		return handleUnknownHost(khPath, hostname, remote, key)
	}
	return fmt.Errorf("主机密钥验证失败 (主机 %s): %w", hostname, err)
}

// handleUnknownHost handles the first-seen host case.
// Interactive → ask user; non-interactive → reject.
func handleUnknownHost(khPath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	hostTrustPromptMu.Lock()
	defer hostTrustPromptMu.Unlock()

	// Another concurrent connection may have trusted this host while waiting.
	if cb, err := knownhosts.New(khPath); err == nil {
		if err := cb(hostname, remote, key); err == nil {
			return nil
		}
	}

	fingerprint := ssh.FingerprintSHA256(key)
	hostKeyType := key.Type()

	if !ui.IsTerminal() {
		return fmt.Errorf(
			"无法验证主机 %s 的身份。\n"+
				"该主机的密钥指纹为:\n"+
				"  类型: %s\n"+
				"  SHA256: %s\n"+
				"请在交互终端中首次连接以信任此密钥，或手动将其添加到 %s",
			hostname, hostKeyType, fingerprint, khPath,
		)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "╔══════════════════════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║  首次连接主机: %-39s ║\n", hostname)
	fmt.Fprintf(os.Stderr, "╠══════════════════════════════════════════════════════════╣\n")
	fmt.Fprintf(os.Stderr, "║  主机密钥类型:  %-40s ║\n", hostKeyType)
	fmt.Fprintf(os.Stderr, "║  SHA256 指纹:                                      ║\n")
	fmt.Fprintf(os.Stderr, "║  %-52s ║\n", fingerprint[:min(52, len(fingerprint))])
	if len(fingerprint) > 52 {
		fmt.Fprintf(os.Stderr, "║  %-52s ║\n", fingerprint[52:])
	}
	fmt.Fprintf(os.Stderr, "╚══════════════════════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "\n")

	if ui.ReadYesNo("是否信任该主机密钥并继续连接？[y/N]: ") {
		if err := appendToKnownHosts(khPath, hostname, remote, key); err != nil {
			return fmt.Errorf("无法保存主机密钥: %w", err)
		}
		fmt.Fprintf(os.Stderr, "主机密钥已保存到 %s\n", khPath)
		return nil
	}
	return fmt.Errorf("用户拒绝了主机 %s 的密钥", hostname)
}

// appendToKnownHosts atomically appends a host key line to known_hosts.
func appendToKnownHosts(khPath, hostname string, remote net.Addr, key ssh.PublicKey) error {
	if err := ensureSSHDir(); err != nil {
		return err
	}

	line := knownhosts.Line([]string{normalizedKnownHost(hostname, remote)}, key)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	return safefile.WithLock(khPath, func() error {
		existing, err := os.ReadFile(khPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("读取 known_hosts 失败: %w", err)
		}
		return safefile.Write(khPath, append(existing, []byte(line)...), 0600, false)
	})
}

func normalizedKnownHost(hostname string, remote net.Addr) string {
	if _, port, err := net.SplitHostPort(hostname); err == nil {
		if port == "22" {
			host, _, _ := net.SplitHostPort(hostname)
			return host
		}
		return knownhosts.Normalize(hostname)
	}
	if tcp, ok := remote.(*net.TCPAddr); ok && tcp.Port != 22 {
		return knownhosts.Normalize(net.JoinHostPort(hostname, strconv.Itoa(tcp.Port)))
	}
	return hostname
}

// NativeConnectPassword connects using Go native SSH with password auth.
func NativeConnectPassword(h config.Host, password string) error {
	return NativeConnectAuth(h, ssh.Password(password), "密码")
}

// NativeConnectAuth connects using an in-memory SSH authentication method.
func NativeConnectAuth(h config.Host, auth ssh.AuthMethod, label string) error {
	hostKeyCB, err := createHostKeyCallback()
	if err != nil {
		return fmt.Errorf("主机密钥回调失败: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            h.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCB,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("%s认证失败: %w", label, err)
	}
	defer client.Close()

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

// NativeExec runs a command using Go native SSH with password auth.
func NativeExec(h config.Host, password string, command string) (string, error) {
	return NativeExecContext(context.Background(), h, password, command)
}

// NativeExecContext runs a password-authenticated command with cancellation.
func NativeExecContext(ctx context.Context, h config.Host, password string, command string) (string, error) {
	return NativeExecAuthContext(ctx, h, ssh.Password(password), "密码", command)
}

// NativeExecAuthContext runs a command using an in-memory SSH authentication method.
func NativeExecAuthContext(ctx context.Context, h config.Host, auth ssh.AuthMethod, label string, command string) (string, error) {
	hostKeyCB, err := createHostKeyCallback()
	if err != nil {
		return "", fmt.Errorf("主机密钥回调失败: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            h.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCB,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return "", fmt.Errorf("%s认证失败: %w", label, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("%s认证 - 创建会话失败: %w", label, err)
	}
	defer session.Close()

	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := session.CombinedOutput(command)
		done <- result{output: output, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		return "", fmt.Errorf("远程命令超时或取消: %w", ctx.Err())
	case result := <-done:
		return string(result.output), result.err
	}
}

// NativePing checks connectivity using Go native SSH with password auth.
func NativePing(h config.Host, password string) (bool, string) {
	return NativePingAuth(h, ssh.Password(password), "密码")
}

// NativePingAuth checks connectivity using an in-memory SSH authentication method.
func NativePingAuth(h config.Host, auth ssh.AuthMethod, label string) (bool, string) {
	hostKeyCB, err := createHostKeyCallback()
	if err != nil {
		return false, err.Error()
	}

	sshConfig := &ssh.ClientConfig{
		User:            h.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCB,
		Timeout:         5 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return false, fmt.Sprintf("%s认证失败: %v", label, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return false, err.Error()
	}
	defer session.Close()

	output, err := session.CombinedOutput("echo ok")
	return err == nil, string(output)
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

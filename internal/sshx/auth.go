package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"golang.org/x/crypto/ssh"
)

// DialContext establishes an authenticated SSH client using sshm's trust policy.
func DialContext(ctx context.Context, h config.Host, store *secret.FileStore) (*ssh.Client, string, error) {
	targetConfig, label, err := clientConfig(h, store)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageAuth, err)
	}
	return dialConfiguredContext(ctx, h, store, targetConfig, label)
}

// DialPasswordContext uses a temporary password for the target without saving it.
func DialPasswordContext(ctx context.Context, h config.Host, store *secret.FileStore, password string) (*ssh.Client, string, error) {
	callback, err := createHostKeyCallback(h)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageTrust, err)
	}
	targetConfig := &ssh.ClientConfig{
		User: h.User, Auth: []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: callback, Timeout: 10 * time.Second,
	}
	return dialConfiguredContext(ctx, h, store, targetConfig, "临时密码")
}

func ConnectTemporaryPassword(h config.Host, store *secret.FileStore, password string) error {
	client, label, err := DialPasswordContext(context.Background(), h, store, password)
	if err != nil {
		return err
	}
	defer client.Close()
	return interactiveSession(client, label)
}

func dialConfiguredContext(ctx context.Context, h config.Host, store *secret.FileStore, targetConfig *ssh.ClientConfig, label string) (*ssh.Client, string, error) {
	if h.JumpHost == "" {
		return dialDirectWithConfig(ctx, h, targetConfig, label)
	}
	if h.ConfigPath == "" {
		return nil, "", operation.Wrap(operation.StageJump, fmt.Errorf("跳板机连接需要主机来自 sshm 配置"))
	}
	jump, _, _, err := config.NewStoreWithPath(h.ConfigPath).FindHost(h.JumpHost)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageJump, fmt.Errorf("解析跳板机失败: %w", err))
	}
	if jump.JumpHost != "" {
		return nil, "", operation.Wrap(operation.StageJump, fmt.Errorf("仅支持单级跳板机，%s 不能再引用跳板机", jump.Alias))
	}
	jumpClient, _, err := dialDirectContext(ctx, *jump, store)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageJump, fmt.Errorf("跳板机 %s 连接失败: %w", jump.Alias, err))
	}
	conn, err := dialThroughJumpContext(ctx, jumpClient, "tcp", fmt.Sprintf("%s:%d", h.Host, h.Port))
	if err != nil {
		jumpClient.Close()
		return nil, "", operation.Wrap(operation.StageJump, fmt.Errorf("通过跳板机连接目标失败: %w", err))
	}
	client, err := newSSHClientContext(ctx, conn, fmt.Sprintf("%s:%d", h.Host, h.Port), targetConfig)
	if err != nil {
		conn.Close()
		jumpClient.Close()
		return nil, "", operation.Wrap(operation.StageAuth, fmt.Errorf("目标 SSH 握手失败: %w", err))
	}
	go func() {
		_ = client.Wait()
		_ = jumpClient.Close()
	}()
	return client, label, nil
}

func dialDirectContext(ctx context.Context, h config.Host, store *secret.FileStore) (*ssh.Client, string, error) {
	sshConfig, label, err := clientConfig(h, store)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageAuth, err)
	}
	return dialDirectWithConfig(ctx, h, sshConfig, label)
}

func dialDirectWithConfig(ctx context.Context, h config.Host, sshConfig *ssh.ClientConfig, label string) (*ssh.Client, string, error) {
	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		stage := operation.StageNetwork
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			stage = operation.StageResolve
		}
		return nil, "", operation.Wrap(stage, fmt.Errorf("网络连接失败: %w", err))
	}
	client, err := newSSHClientContext(ctx, conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		stage := operation.StageAuth
		if operation.StageOf(err, stage) == operation.StageTrust {
			stage = operation.StageTrust
		}
		return nil, "", operation.Wrap(stage, fmt.Errorf("%s认证或握手失败: %w", label, err))
	}
	return client, label, nil
}

type netConnResult struct {
	conn net.Conn
	err  error
}

func dialThroughJumpContext(ctx context.Context, client *ssh.Client, network, addr string) (net.Conn, error) {
	done := make(chan netConnResult, 1)
	go func() {
		conn, err := client.Dial(network, addr)
		done <- netConnResult{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		return nil, ctx.Err()
	case result := <-done:
		return result.conn, result.err
	}
}

type sshClientResult struct {
	client *ssh.Client
	err    error
}

func newSSHClientContext(ctx context.Context, conn net.Conn, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	done := make(chan sshClientResult, 1)
	go func() {
		clientConn, channels, requests, err := ssh.NewClientConn(conn, addr, config)
		if err != nil {
			done <- sshClientResult{err: err}
			return
		}
		done <- sshClientResult{client: ssh.NewClient(clientConn, channels, requests)}
	}()
	select {
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	case result := <-done:
		return result.client, result.err
	}
}

func clientConfig(h config.Host, store *secret.FileStore) (*ssh.ClientConfig, string, error) {
	callback, err := createHostKeyCallback(h)
	if err != nil {
		return nil, "", operation.Wrap(operation.StageTrust, err)
	}
	var methods []ssh.AuthMethod
	var labels []string
	strategy := GetAuthStrategy(h.Auth)
	if strategy != AuthPassword && isManagedIdentity(h) {
		_, signer, err := managedKeyMaterial(h, store)
		if err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
			labels = append(labels, "托管密钥")
		} else if strategy == AuthKey {
			return nil, "", err
		}
	}
	if strategy != AuthKey && h.PasswordRef != "" && store != nil {
		password, err := store.GetPassword(h.PasswordRef)
		if err == nil {
			methods = append(methods, ssh.Password(password))
			labels = append(labels, "密码")
		} else if strategy == AuthPassword {
			return nil, "", err
		}
	}
	if len(methods) == 0 {
		return nil, "", fmt.Errorf("主机 %s 未配置可用的认证凭据", h.Alias)
	}
	return &ssh.ClientConfig{
		User:            h.User,
		Auth:            methods,
		HostKeyCallback: callback,
		Timeout:         10 * time.Second,
	}, strings.Join(labels, "/"), nil
}

// Connect dials an interactive SSH session using the best available auth.
func Connect(h config.Host, store *secret.FileStore) error {
	client, label, err := DialContext(context.Background(), h, store)
	if err != nil {
		return err
	}
	defer client.Close()
	return operation.Wrap(operation.StageSession, interactiveSession(client, label))
}

// ExecCommand runs a command on a host using the best available auth.
func ExecCommand(h config.Host, store *secret.FileStore, command string) (string, error) {
	return ExecCommandContext(context.Background(), h, store, command)
}

// ExecCommandContext runs a command using a consistent authentication plan.
func ExecCommandContext(ctx context.Context, h config.Host, store *secret.FileStore, command string) (string, error) {
	client, _, err := DialContext(ctx, h, store)
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", operation.Wrap(operation.StageSession, fmt.Errorf("创建会话失败: %w", err))
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
		return string(result.output), operation.Wrap(operation.StageExecute, result.err)
	}
}

// CheckPingContext verifies that a complete SSH session can execute a command.
func CheckPingContext(ctx context.Context, h config.Host, store *secret.FileStore) (string, error) {
	client, _, err := DialContext(ctx, h, store)
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", operation.Wrap(operation.StageSession, fmt.Errorf("创建会话失败: %w", err))
	}
	defer session.Close()
	output, err := session.CombinedOutput("echo ok")
	return string(output), operation.Wrap(operation.StageExecute, err)
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

// ManagedKeyPrivateKey returns the decrypted private key for short-lived
// integrations such as rsync transport setup. Callers must remove any copy.
func ManagedKeyPrivateKey(h config.Host, store *secret.FileStore) ([]byte, error) {
	privateKey, _, err := managedKeyMaterial(h, store)
	return privateKey, err
}

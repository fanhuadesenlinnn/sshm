package command

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var rsyncEndpointPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func tryRsyncTransfer(ctx context.Context, client *ssh.Client, sftpClient *sftp.Client, host config.Host, store *secret.FileStore, options transferOptions) (string, bool, error) {
	rsyncPath, sshPath, ok := rsyncAvailable(client, host, store, options)
	if !ok {
		return "", false, nil
	}
	sshCommand, cleanup, err := prepareRsyncTransportWithTimeout(host, store, sshPath, options.connectTimeout)
	if err != nil {
		return "", false, nil
	}
	defer cleanup()

	if options.direction == "push" {
		return pushRsync(ctx, rsyncPath, sshCommand, sftpClient, host, options)
	}
	return pullRsync(ctx, rsyncPath, sshCommand, sftpClient, host, options)
}

func rsyncAvailable(client *ssh.Client, host config.Host, store *secret.FileStore, options transferOptions) (string, string, bool) {
	if host.JumpHost != "" || store == nil || options.localPath == "" || options.remotePath == "" {
		return "", "", false
	}
	if _, managed := config.ManagedKeyName(host.Identity); !managed {
		return "", "", false
	}
	if !rsyncEndpointPart.MatchString(host.User) || !rsyncEndpointPart.MatchString(host.Host) {
		return "", "", false
	}
	if strings.ContainsAny(options.remotePath, "\r\n\x00") || strings.ContainsAny(options.localPath, "\x00") {
		return "", "", false
	}
	rsyncPath, err := exec.LookPath("rsync")
	if err != nil {
		return "", "", false
	}
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return "", "", false
	}
	session, err := client.NewSession()
	if err != nil {
		return "", "", false
	}
	defer session.Close()
	if err := session.Run("rsync --version >/dev/null 2>&1"); err != nil {
		return "", "", false
	}
	return rsyncPath, sshPath, true
}

func prepareRsyncTransport(host config.Host, store *secret.FileStore, sshPath string) (string, func(), error) {
	return prepareRsyncTransportWithTimeout(host, store, sshPath, 10*time.Second)
}

func prepareRsyncTransportWithTimeout(host config.Host, store *secret.FileStore, sshPath string, connectTimeout time.Duration) (string, func(), error) {
	privateKey, err := sshx.ManagedKeyPrivateKey(host, store)
	if err != nil {
		return "", nil, err
	}
	tempDir, err := os.MkdirTemp("", "sshm-rsync-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	keyPath := filepath.Join(tempDir, "identity")
	if err := os.WriteFile(keyPath, privateKey, 0600); err != nil {
		cleanup()
		return "", nil, err
	}

	timeoutSeconds := int((connectTimeout + time.Second - 1) / time.Second)
	if timeoutSeconds < 1 {
		timeoutSeconds = 10
	}
	args := []string{
		sshPath,
		"-F", nullDevice(),
		"-i", keyPath,
		"-p", fmt.Sprintf("%d", host.Port),
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "NumberOfPasswordPrompts=0",
		"-o", "PasswordAuthentication=no",
		"-o", "PreferredAuthentications=publickey",
		"-o", fmt.Sprintf("ConnectTimeout=%d", timeoutSeconds),
		"-o", "LogLevel=ERROR",
	}
	policy := host.ResolvedHostKeyPolicy
	if policy == "" {
		policy = host.HostKeyPolicy
	}
	if policy == "" {
		policy = config.HostKeyPolicyStrict
	}
	if policy == config.HostKeyPolicyInsecure {
		args = append(args,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile="+nullDevice(),
			"-o", "GlobalKnownHostsFile="+nullDevice(),
		)
		return shellCommand(args), cleanup, nil
	}

	entry, err := trustedHostEntry(host)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(entry.PublicKey))
	if err != nil {
		cleanup()
		return "", nil, err
	}
	address := knownhosts.Normalize(net.JoinHostPort(host.Host, fmt.Sprintf("%d", host.Port)))
	knownHostsPath := filepath.Join(tempDir, "known_hosts")
	if err := os.WriteFile(knownHostsPath, []byte(knownhosts.Line([]string{address}, publicKey)+"\n"), 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	args = append(args,
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+knownHostsPath,
		"-o", "GlobalKnownHostsFile="+nullDevice(),
	)
	return shellCommand(args), cleanup, nil
}

func trustedHostEntry(host config.Host) (config.HostTrustEntry, error) {
	if host.ConfigPath == "" {
		return config.HostTrustEntry{}, fmt.Errorf("主机缺少配置路径")
	}
	doc, err := config.NewRepositoryWithPath(host.ConfigPath).Load()
	if err != nil {
		return config.HostTrustEntry{}, err
	}
	for _, entry := range doc.HostTrust.Entries {
		if entry.Host == host.Host && entry.Port == host.Port {
			return entry, nil
		}
	}
	return config.HostTrustEntry{}, fmt.Errorf("主机 %s 尚无可信密钥", host.Alias)
}

func pushRsync(ctx context.Context, rsyncPath, sshCommand string, client *sftp.Client, host config.Host, options transferOptions) (string, bool, error) {
	if _, err := os.Stat(options.localPath); err != nil {
		return options.remotePath, true, fmt.Errorf("读取本地源失败: %w", err)
	}
	remotePath := path.Clean(options.remotePath)
	if remotePath == "." || remotePath == "/" || strings.HasPrefix(remotePath, "~/") {
		return remotePath, true, fmt.Errorf("远程目标必须是明确路径，且不支持 ~ 展开: %s", remotePath)
	}
	if _, err := client.Stat(remotePath); err == nil {
		if !options.overwrite {
			return remotePath, true, fmt.Errorf("远程目标已存在；使用 --overwrite 明确覆盖")
		}
	} else if !os.IsNotExist(err) {
		return remotePath, true, fmt.Errorf("检查远程目标失败: %w", err)
	}
	temp := remotePath + fmt.Sprintf(".sshm-rsync-tmp-%d-%d", os.Getpid(), time.Now().UnixNano())
	_ = client.RemoveAll(temp)
	if err := runRsync(ctx, rsyncPath, sshCommand, filepath.Clean(options.localPath), rsyncRemote(host, temp)); err != nil {
		if cleanupErr := client.RemoveAll(temp); cleanupErr != nil {
			return remotePath, true, fmt.Errorf("rsync 失败且无法清理远程临时目标: %v；原始错误: %w", cleanupErr, err)
		}
		if options.method == "rsync" {
			return remotePath, true, fmt.Errorf("rsync 推送失败: %w", err)
		}
		return "", false, nil
	}
	if _, err := client.Stat(temp); err != nil {
		_ = client.RemoveAll(temp)
		if options.method == "rsync" {
			return remotePath, true, fmt.Errorf("rsync 推送未产生远程临时目标: %w", err)
		}
		return "", false, nil
	}
	defer client.RemoveAll(temp)
	return remotePath, true, activateRemoteTemp(client, temp, remotePath, options.overwrite)
}

func pullRsync(ctx context.Context, rsyncPath, sshCommand string, client *sftp.Client, host config.Host, options transferOptions) (string, bool, error) {
	remotePath := path.Clean(options.remotePath)
	if _, err := client.Stat(remotePath); err != nil {
		return "", true, fmt.Errorf("读取远程源失败: %w", err)
	}
	name := path.Base(remotePath)
	if name == "." || name == "/" {
		return "", true, fmt.Errorf("远程源必须是明确文件或目录")
	}
	destination := filepath.Join(options.localPath, host.Alias, name)
	if _, err := os.Stat(destination); err == nil {
		if !options.overwrite {
			return destination, true, fmt.Errorf("本地目标已存在；使用 --overwrite 明确覆盖")
		}
	} else if !os.IsNotExist(err) {
		return destination, true, fmt.Errorf("检查本地目标失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return destination, true, err
	}
	temp := destination + fmt.Sprintf(".sshm-rsync-tmp-%d-%d", os.Getpid(), time.Now().UnixNano())
	_ = os.RemoveAll(temp)
	if err := runRsync(ctx, rsyncPath, sshCommand, rsyncRemote(host, remotePath), temp); err != nil {
		if cleanupErr := os.RemoveAll(temp); cleanupErr != nil {
			return destination, true, fmt.Errorf("rsync 失败且无法清理本地临时目标: %v；原始错误: %w", cleanupErr, err)
		}
		if options.method == "rsync" {
			return destination, true, fmt.Errorf("rsync 拉取失败: %w", err)
		}
		return "", false, nil
	}
	if _, err := os.Stat(temp); err != nil {
		_ = os.RemoveAll(temp)
		if options.method == "rsync" {
			return destination, true, fmt.Errorf("rsync 拉取未产生本地临时目标: %w", err)
		}
		return "", false, nil
	}
	defer os.RemoveAll(temp)
	return destination, true, activateLocalTemp(temp, destination, options.overwrite)
}

func runRsync(ctx context.Context, rsyncPath, sshCommand, source, destination string) error {
	command := exec.CommandContext(ctx, rsyncPath,
		"--archive",
		"--protect-args",
		"-e", sshCommand,
		"--",
		source,
		destination,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if len(message) > 4096 {
			message = message[len(message)-4096:]
		}
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

func rsyncRemote(host config.Host, remotePath string) string {
	return fmt.Sprintf("%s@%s:%s", host.User, host.Host, remotePath)
}

func shellCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func nullDevice() string {
	if os.PathSeparator == '\\' {
		return "NUL"
	}
	return "/dev/null"
}

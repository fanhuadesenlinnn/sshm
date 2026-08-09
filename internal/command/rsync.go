package command

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/safefile"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var rsyncEndpointPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func tryRsyncTransfer(ctx context.Context, client *ssh.Client, sftpClient *sftp.Client, host config.Host, store *secret.FileStore, options transferOptions) (string, bool, bool, error) {
	rsyncPath, sshPath, ok := rsyncAvailable(ctx, client, host, store, options)
	if !ok {
		if options.method == "rsync" {
			return "", false, true, fmt.Errorf("显式 rsync 不可用或无法保证 v6 安全语义")
		}
		return "", false, false, nil
	}
	sshCommand, cleanup, err := prepareRsyncTransportWithTimeout(host, store, sshPath, options.connectTimeout)
	if err != nil {
		if options.method == "rsync" {
			return "", false, true, err
		}
		return "", false, false, nil
	}
	defer cleanup()

	var destination string
	var changed bool
	if options.direction == "push" {
		destination, changed, err = pushRsync(ctx, rsyncPath, sshCommand, sftpClient, host, options)
	} else {
		destination, changed, err = pullRsync(ctx, rsyncPath, sshCommand, sftpClient, host, options)
	}
	var fallback *rsyncFallbackError
	if errors.As(err, &fallback) && options.method == "auto" {
		return "", false, false, nil
	}
	return destination, changed, true, err
}

func rsyncAvailable(ctx context.Context, client *ssh.Client, host config.Host, store *secret.FileStore, options transferOptions) (string, string, bool) {
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
	if _, err := sshx.ExecCommandOnClient(ctx, client, "rsync --version >/dev/null 2>&1", nil, nil); err != nil {
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
	tempDir, err := os.MkdirTemp("", "sshmd-rsync-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	if err := safefile.Restrict(tempDir, 0700); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("保护 rsync 临时目录失败: %w", err)
	}
	keyPath := filepath.Join(tempDir, "identity")
	if err := safefile.Write(keyPath, privateKey, 0600); err != nil {
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
		return shellquote.Command(args), cleanup, nil
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
	if err := safefile.Write(knownHostsPath, []byte(knownhosts.Line([]string{address}, publicKey)+"\n"), 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	args = append(args,
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+knownHostsPath,
		"-o", "GlobalKnownHostsFile="+nullDevice(),
	)
	return shellquote.Command(args), cleanup, nil
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
	sourceManifest, err := localManifest(options.localPath)
	if err != nil {
		return options.remotePath, false, err
	}
	remotePath, err := cleanTransferRemotePath(options.remotePath)
	if err != nil {
		return options.remotePath, false, err
	}
	exists := false
	var targetManifest []manifestEntry
	if _, err := client.Lstat(remotePath); err == nil {
		exists = true
		if options.validateChecksum {
			targetManifest, err = remoteManifest(client, remotePath)
			if err != nil {
				return remotePath, false, err
			}
			if manifestsEqual(sourceManifest, targetManifest) {
				return remotePath, false, nil
			}
		}
		if options.diffWriter != nil {
			if targetManifest == nil {
				targetManifest, err = remoteManifest(client, remotePath)
				if err != nil {
					return remotePath, false, err
				}
			}
			if err := writePushDiff(options.diffWriter, client, options.localPath, remotePath, sourceManifest, targetManifest); err != nil {
				return remotePath, false, err
			}
		}
		if !options.check && !options.overwrite && !options.backup {
			return remotePath, false, fmt.Errorf("远程目标已存在且内容不同；使用 --overwrite 或 --backup")
		}
	} else if !os.IsNotExist(err) {
		return remotePath, false, fmt.Errorf("检查远程目标失败: %w", err)
	} else if options.diffWriter != nil {
		if err := writePushDiff(options.diffWriter, client, options.localPath, remotePath, sourceManifest, nil); err != nil {
			return remotePath, false, err
		}
	}
	if options.check {
		return remotePath, true, nil
	}
	temp := remotePath + fmt.Sprintf(".sshmd-rsync-tmp-%d-%d", os.Getpid(), time.Now().UnixNano())
	_ = client.RemoveAll(temp)
	defer client.RemoveAll(temp)
	source := rsyncDirectorySource(filepath.Clean(options.localPath), string(os.PathSeparator), sourceManifest)
	if err := runRsync(ctx, rsyncPath, sshCommand, source, rsyncRemote(host, temp)); err != nil {
		return remotePath, false, &rsyncFallbackError{err: fmt.Errorf("rsync 推送失败: %w", err)}
	}
	if options.validateChecksum {
		tempManifest, err := remoteManifest(client, temp)
		if err != nil {
			return remotePath, false, &rsyncFallbackError{err: err}
		}
		if !manifestsEqual(sourceManifest, tempManifest) {
			return remotePath, false, &rsyncFallbackError{err: fmt.Errorf("rsync 远程临时目标 checksum 校验失败")}
		}
	}
	return remotePath, true, activateRemoteTemp(client, temp, remotePath, exists, options.overwrite, options.backup)
}

func pullRsync(ctx context.Context, rsyncPath, sshCommand string, client *sftp.Client, host config.Host, options transferOptions) (string, bool, error) {
	remotePath, err := cleanTransferRemotePath(options.remotePath)
	if err != nil {
		return options.localPath, false, err
	}
	sourceManifest, err := remoteManifest(client, remotePath)
	if err != nil {
		return options.localPath, false, err
	}
	destination := options.localPath
	if err := validateRemoteManifestDestinations(
		destination, sourceManifest, runtime.GOOS == "windows", localPathComparisonCaseInsensitive(),
	); err != nil {
		return destination, false, err
	}
	exists := false
	if _, err := os.Lstat(destination); err == nil {
		exists = true
		if options.validateChecksum {
			targetManifest, err := localManifest(destination)
			if err != nil {
				return destination, false, err
			}
			if manifestsEqual(sourceManifest, targetManifest) {
				return destination, false, nil
			}
		}
		if !options.check && !options.overwrite && !options.backup {
			return destination, false, fmt.Errorf("本地目标已存在且内容不同；使用 --overwrite 或 --backup")
		}
	} else if !os.IsNotExist(err) {
		return destination, false, fmt.Errorf("检查本地目标失败: %w", err)
	}
	if options.check {
		return destination, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return destination, false, err
	}
	temp := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+fmt.Sprintf(".sshmd-rsync-tmp-%d-%d", os.Getpid(), time.Now().UnixNano()))
	_ = os.RemoveAll(temp)
	defer os.RemoveAll(temp)
	remoteSource := rsyncDirectorySource(remotePath, "/", sourceManifest)
	if err := runRsync(ctx, rsyncPath, sshCommand, rsyncRemote(host, remoteSource), temp); err != nil {
		return destination, false, &rsyncFallbackError{err: fmt.Errorf("rsync 拉取失败: %w", err)}
	}
	if options.validateChecksum {
		tempManifest, err := localManifest(temp)
		if err != nil {
			return destination, false, &rsyncFallbackError{err: err}
		}
		if !manifestsEqual(sourceManifest, tempManifest) {
			return destination, false, &rsyncFallbackError{err: fmt.Errorf("rsync 本地临时目标 checksum 校验失败")}
		}
	}
	return destination, true, activateLocalTemp(temp, destination, exists, options.overwrite, options.backup)
}

func manifestRootIsDirectory(manifest []manifestEntry) bool {
	for _, entry := range manifest {
		if entry.Path == "." {
			return entry.Type == "dir"
		}
	}
	return false
}

func rsyncDirectorySource(source, separator string, manifest []manifestEntry) string {
	if manifestRootIsDirectory(manifest) && !strings.HasSuffix(source, separator) {
		return source + separator
	}
	return source
}

type rsyncFallbackError struct {
	err error
}

func (e *rsyncFallbackError) Error() string { return e.err.Error() }
func (e *rsyncFallbackError) Unwrap() error { return e.err }

func runRsync(ctx context.Context, rsyncPath, sshCommand, source, destination string) error {
	// #nosec G204 -- rsync/ssh paths are resolved with LookPath; data args are passed without a shell.
	command := exec.CommandContext(ctx, rsyncPath,
		"--archive",
		"--protect-args",
		"-e", sshCommand,
		"--",
		source,
		destination,
	)
	output := newBoundedTailBuffer(64 * 1024)
	command.Stdout = output
	command.Stderr = output
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

type boundedTailBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func newBoundedTailBuffer(limit int) *boundedTailBuffer {
	if limit < 1 {
		limit = 1
	}
	return &boundedTailBuffer{limit: limit}
}

func (b *boundedTailBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(data)
	if len(data) >= b.limit {
		b.data = append(b.data[:0], data[len(data)-b.limit:]...)
		b.truncated = true
		return written, nil
	}
	if overflow := len(b.data) + len(data) - b.limit; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
		b.truncated = true
	}
	b.data = append(b.data, data...)
	return written, nil
}

func (b *boundedTailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return "[rsync 输出已截断，仅保留末尾]\n" + string(b.data)
	}
	return string(b.data)
}

func rsyncRemote(host config.Host, remotePath string) string {
	return fmt.Sprintf("%s@%s:%s", host.User, host.Host, remotePath)
}

func nullDevice() string {
	if os.PathSeparator == '\\' {
		return "NUL"
	}
	return "/dev/null"
}

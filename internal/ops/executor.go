package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type ExecOptions struct {
	Command        string
	ConnectTimeout time.Duration
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
}

type TransferOptions struct {
	Direction        string
	Src              string
	Dest             string
	Method           string
	Overwrite        bool
	Backup           bool
	ValidateChecksum bool
	DestinationExact bool
	Check            bool
	Diff             bool
	ConnectTimeout   time.Duration
}

type Result struct {
	Host        config.Host
	OK          bool
	Stage       operation.FailureStage
	Output      string
	Method      string
	Destination string
	Changed     bool
	WouldChange bool
	Duration    time.Duration
	Err         error
}

// RemoteFileInfo describes a remote filesystem entry observed over SFTP.
type RemoteFileInfo struct {
	Exists bool
	IsDir  bool
	IsLink bool
	Mode   os.FileMode
	Size   int64
	Target string
}

// BecomeCommand wraps a remote command for execution through sudo as a fixed
// user. Both arguments are shell-quoted so the resulting command line is
// safe to pass through a remote shell.
func BecomeCommand(command, user string) string {
	if user == "" {
		user = "root"
	}
	return "sudo -n -u " + shellquote.Single(user) + " -- sh -c " + shellquote.Single(command)
}

type Executor interface {
	Exec(context.Context, config.Host, ExecOptions) Result
	Push(context.Context, config.Host, TransferOptions) Result
	Pull(context.Context, config.Host, TransferOptions) Result
	Stat(context.Context, config.Host, string, time.Duration) (RemoteFileInfo, error)
	// DialTCP opens a TCP connection from the target host's side by dialing
	// through an SSH session.
	DialTCP(context.Context, config.Host, string, time.Duration) (net.Conn, error)
}

type TransferFunc func(context.Context, config.Host, TransferOptions) Result

// DialFunc establishes a reusable SSH client for a host.
type DialFunc func(ctx context.Context, host config.Host, connectTimeout time.Duration) (sshx.Client, error)

// SFTPConn is the subset of the SFTP client used by Stat, so tests can inject
// a fake instead of dialing a real host.
type SFTPConn interface {
	Lstat(path string) (os.FileInfo, error)
	ReadLink(path string) (string, error)
	Close() error
}

// NativeExecutor implements Executor over reusable SSH sessions. Every
// operation acquires its connection through the session manager, which is the
// single dial/SFTP path: connection reuse and failure-reconnect are enforced
// here rather than at individual call sites.
type NativeExecutor struct {
	Secrets  *secret.FileStore
	PushFunc TransferFunc
	PullFunc TransferFunc
	DialFunc DialFunc
	SFTPOpen SFTPOpenFunc

	managerMu sync.Mutex
	manager   *sessionManager
}

func (e *NativeExecutor) sessionManager() *sessionManager {
	e.managerMu.Lock()
	defer e.managerMu.Unlock()
	if e.manager == nil {
		dial := e.DialFunc
		if dial == nil {
			dial = func(ctx context.Context, host config.Host, connectTimeout time.Duration) (sshx.Client, error) {
				client, _, err := sshx.DialContextWithTimeout(ctx, host, e.Secrets, connectTimeout)
				return client, err
			}
		}
		sftpOpen := e.SFTPOpen
		if sftpOpen == nil {
			sftpOpen = defaultSFTPOpen
		}
		e.manager = newSessionManager(dial, sftpOpen)
	}
	return e.manager
}

func (e *NativeExecutor) Exec(ctx context.Context, host config.Host, options ExecOptions) Result {
	start := time.Now()
	manager := e.sessionManager()
	session, release, err := manager.beginOperation(ctx, host, options.ConnectTimeout)
	if err != nil {
		err = contextError(ctx, "执行远程命令", err)
		return newResult(host, "", "", "", err, operation.StageExecute, time.Since(start))
	}
	defer release()
	output, err := sshx.ExecCommandOnClientWithStdin(ctx, session.client, options.Command, options.Stdin, options.Stdout, options.Stderr)
	if err != nil && isBrokenConnection(err) {
		manager.invalidate(host.Alias, session)
	}
	return newResult(host, output, "", "", err, operation.StageExecute, time.Since(start))
}

func (e *NativeExecutor) Push(ctx context.Context, host config.Host, options TransferOptions) Result {
	if e.PushFunc == nil {
		return newResult(host, "", "", "", operation.Wrap(operation.StageTransfer, errTransferUnavailable), operation.StageTransfer, 0)
	}
	if err := ctx.Err(); err != nil {
		return canceledTransferResult(host, err)
	}
	result := e.PushFunc(ctx, host, options)
	if err := ctx.Err(); err != nil {
		return markTransferCanceled(result, err)
	}
	return result
}

func (e *NativeExecutor) Pull(ctx context.Context, host config.Host, options TransferOptions) Result {
	if e.PullFunc == nil {
		return newResult(host, "", "", "", operation.Wrap(operation.StageTransfer, errTransferUnavailable), operation.StageTransfer, 0)
	}
	if err := ctx.Err(); err != nil {
		return canceledTransferResult(host, err)
	}
	result := e.PullFunc(ctx, host, options)
	if err := ctx.Err(); err != nil {
		return markTransferCanceled(result, err)
	}
	return result
}

// Stat returns lstat information for a remote path, reusing the cached SSH
// session and its shared SFTP channel. A missing path yields an empty
// RemoteFileInfo with nil error so modules can branch on existence.
func (e *NativeExecutor) Stat(ctx context.Context, host config.Host, path string, connectTimeout time.Duration) (RemoteFileInfo, error) {
	manager := e.sessionManager()
	session, release, err := manager.beginOperation(ctx, host, connectTimeout)
	if err != nil {
		return RemoteFileInfo{}, contextError(ctx, "获取远程文件状态", err)
	}
	defer release()
	stop := context.AfterFunc(ctx, func() { manager.invalidate(host.Alias, session) })
	defer stop()
	client, err := manager.sftpFor(ctx, host.Alias, session)
	if err != nil {
		return RemoteFileInfo{}, contextError(ctx, "获取远程文件状态", err)
	}
	info, err := client.Lstat(path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		manager.invalidate(host.Alias, session)
		return RemoteFileInfo{}, contextError(ctx, "获取远程文件状态", ctxErr)
	}
	if err != nil {
		if isNoSuchFile(err) {
			return RemoteFileInfo{}, nil
		}
		return RemoteFileInfo{}, err
	}
	out := RemoteFileInfo{
		Exists: true,
		IsDir:  info.IsDir(),
		IsLink: info.Mode()&os.ModeSymlink != 0,
		Mode:   info.Mode(),
		Size:   info.Size(),
	}
	if out.IsLink {
		if target, readErr := client.ReadLink(path); readErr == nil {
			out.Target = target
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		manager.invalidate(host.Alias, session)
		return RemoteFileInfo{}, contextError(ctx, "获取远程文件状态", ctxErr)
	}
	return out, nil
}

// CloseSessions closes every cached SSH connection. Long-lived processes such
// as the interactive workbench should defer CloseSessions.
func (e *NativeExecutor) CloseSessions() {
	e.sessionManager().closeAll()
}

// ReusableSession returns cached SSH/SFTP clients with an exclusive per-host
// operation lease. Callers must invoke release exactly once; passing true
// invalidates this session generation after a failed transfer.
func (e *NativeExecutor) ReusableSession(ctx context.Context, host config.Host, connectTimeout time.Duration) (*ssh.Client, *sftp.Client, func(bool), error) {
	manager := e.sessionManager()
	session, endOperation, err := manager.beginOperation(ctx, host, connectTimeout)
	if err != nil {
		return nil, nil, nil, contextError(ctx, "获取传输会话", err)
	}
	client, ok := session.client.(*ssh.Client)
	if !ok {
		manager.invalidate(host.Alias, session)
		endOperation()
		return nil, nil, nil, fmt.Errorf("会话不支持传输复用")
	}
	conn, err := manager.sftpFor(ctx, host.Alias, session)
	if err != nil {
		endOperation()
		return nil, nil, nil, contextError(ctx, "打开 SFTP 会话", err)
	}
	sftpClient, ok := conn.(*sftp.Client)
	if !ok {
		manager.invalidate(host.Alias, session)
		endOperation()
		return nil, nil, nil, fmt.Errorf("会话不支持传输复用")
	}
	stopCancellation := context.AfterFunc(ctx, func() { manager.invalidate(host.Alias, session) })
	var releaseOnce sync.Once
	release := func(invalidate bool) {
		releaseOnce.Do(func() {
			_ = stopCancellation()
			if invalidate || ctx.Err() != nil {
				manager.invalidate(host.Alias, session)
			}
			endOperation()
		})
	}
	return client, sftpClient, release, nil
}

// DialTCP dials address from the target host's side through the cached SSH
// session, so reachability checks work through jump hosts and private nets.
func (e *NativeExecutor) DialTCP(ctx context.Context, host config.Host, address string, connectTimeout time.Duration) (net.Conn, error) {
	manager := e.sessionManager()
	session, release, err := manager.beginOperation(ctx, host, connectTimeout)
	if err != nil {
		return nil, contextError(ctx, "建立目标侧 TCP 连接", err)
	}
	defer release()
	stop := context.AfterFunc(ctx, func() { manager.invalidate(host.Alias, session) })
	defer stop()
	type dialResult struct {
		conn net.Conn
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, dialErr := session.client.Dial("tcp", address)
		if ctx.Err() != nil && conn != nil {
			_ = conn.Close()
			conn = nil
		}
		done <- dialResult{conn: conn, err: dialErr}
	}()
	select {
	case <-ctx.Done():
		manager.invalidate(host.Alias, session)
		return nil, ctx.Err()
	case result := <-done:
		if ctxErr := ctx.Err(); ctxErr != nil {
			if result.conn != nil {
				_ = result.conn.Close()
			}
			manager.invalidate(host.Alias, session)
			return nil, ctxErr
		}
		if result.err != nil && isBrokenConnection(result.err) {
			manager.invalidate(host.Alias, session)
		}
		return result.conn, result.err
	}
}

func contextError(ctx context.Context, action string, fallback error) error {
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.StageTimeout, fmt.Errorf("%s超时或取消: %w", action, err))
	}
	return fallback
}

func canceledTransferResult(host config.Host, err error) Result {
	wrapped := operation.Wrap(operation.StageTimeout, fmt.Errorf("文件传输超时或取消: %w", err))
	return newResult(host, "", "", "", wrapped, operation.StageTimeout, 0)
}

func markTransferCanceled(result Result, err error) Result {
	result.OK = false
	result.Stage = operation.StageTimeout
	result.Err = operation.Wrap(operation.StageTimeout, fmt.Errorf("文件传输超时或取消: %w", err))
	return result
}

// isBrokenConnection reports whether an error means the cached SSH connection
// itself is unusable (network or session level), so it should be dropped and
// re-dialed for the next operation.
func isBrokenConnection(err error) bool {
	stage := operation.StageOf(err, operation.StageUnknown)
	return stage == operation.StageNetwork || stage == operation.StageSession
}

func isNoSuchFile(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var status *sftp.StatusError
	if errors.As(err, &status) {
		return status.Code == uint32(sftp.ErrSSHFxNoSuchFile)
	}
	return false
}

func NewTransferResult(host config.Host, method, destination string, err error, duration time.Duration) Result {
	return NewTransferResultWithChange(host, method, destination, false, err, duration)
}

func NewTransferResultWithChange(host config.Host, method, destination string, changed bool, err error, duration time.Duration) Result {
	result := newResult(host, "", method, destination, err, operation.StageTransfer, duration)
	result.Changed = changed
	return result
}

func newResult(host config.Host, output, method, destination string, err error, fallback operation.FailureStage, duration time.Duration) Result {
	return Result{
		Host: host, OK: err == nil, Stage: operation.StageOf(err, fallback), Output: output,
		Method: method, Destination: destination, Duration: duration, Err: err,
	}
}

type unavailableError string

func (e unavailableError) Error() string { return string(e) }

const errTransferUnavailable unavailableError = "当前执行器未配置文件传输能力"

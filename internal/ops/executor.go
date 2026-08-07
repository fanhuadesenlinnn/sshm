package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/pkg/sftp"
)

type ExecOptions struct {
	Command        string
	ConnectTimeout time.Duration
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

type Executor interface {
	Exec(context.Context, config.Host, ExecOptions) Result
	Push(context.Context, config.Host, TransferOptions) Result
	Pull(context.Context, config.Host, TransferOptions) Result
	Stat(context.Context, config.Host, string, time.Duration) (RemoteFileInfo, error)
}

type TransferFunc func(context.Context, config.Host, TransferOptions) Result

type NativeExecutor struct {
	Secrets  *secret.FileStore
	PushFunc TransferFunc
	PullFunc TransferFunc
}

func (e *NativeExecutor) Exec(ctx context.Context, host config.Host, options ExecOptions) Result {
	start := time.Now()
	var output string
	var err error
	if options.Stdout != nil || options.Stderr != nil {
		output, err = sshx.ExecCommandStreamWithConnectTimeoutContext(ctx, host, e.Secrets, options.Command, options.ConnectTimeout, options.Stdout, options.Stderr)
	} else {
		output, err = sshx.ExecCommandWithConnectTimeoutContext(ctx, host, e.Secrets, options.Command, options.ConnectTimeout)
	}
	return newResult(host, output, "", "", err, operation.StageExecute, time.Since(start))
}

func (e *NativeExecutor) Push(ctx context.Context, host config.Host, options TransferOptions) Result {
	if e.PushFunc == nil {
		return newResult(host, "", "", "", operation.Wrap(operation.StageTransfer, errTransferUnavailable), operation.StageTransfer, 0)
	}
	return e.PushFunc(ctx, host, options)
}

func (e *NativeExecutor) Pull(ctx context.Context, host config.Host, options TransferOptions) Result {
	if e.PullFunc == nil {
		return newResult(host, "", "", "", operation.Wrap(operation.StageTransfer, errTransferUnavailable), operation.StageTransfer, 0)
	}
	return e.PullFunc(ctx, host, options)
}

// Stat returns lstat information for a remote path. A missing path yields an
// empty RemoteFileInfo with nil error so modules can branch on existence.
func (e *NativeExecutor) Stat(ctx context.Context, host config.Host, path string, connectTimeout time.Duration) (RemoteFileInfo, error) {
	client, _, err := sshx.DialContextWithTimeout(ctx, host, e.Secrets, connectTimeout)
	if err != nil {
		return RemoteFileInfo{}, err
	}
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return RemoteFileInfo{}, fmt.Errorf("启动 SFTP 失败: %w", err)
	}
	defer sftpClient.Close()
	info, err := sftpClient.Lstat(path)
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
		if target, readErr := sftpClient.ReadLink(path); readErr == nil {
			out.Target = target
		}
	}
	return out, nil
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

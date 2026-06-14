package ops

import (
	"context"
	"io"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/sshx"
)

type ExecOptions struct {
	Command        string
	ConnectTimeout time.Duration
	Stdout         io.Writer
	Stderr         io.Writer
}

type TransferOptions struct {
	Direction      string
	Src            string
	Dest           string
	Method         string
	Overwrite      bool
	ConnectTimeout time.Duration
}

type Result struct {
	Host        config.Host
	OK          bool
	Stage       operation.FailureStage
	Output      string
	Method      string
	Destination string
	Duration    time.Duration
	Err         error
}

type Executor interface {
	Exec(context.Context, config.Host, ExecOptions) Result
	Push(context.Context, config.Host, TransferOptions) Result
	Pull(context.Context, config.Host, TransferOptions) Result
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

func NewTransferResult(host config.Host, method, destination string, err error, duration time.Duration) Result {
	return newResult(host, "", method, destination, err, operation.StageTransfer, duration)
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

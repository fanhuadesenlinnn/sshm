package command

import (
	"context"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ops"
)

func (app *App) operationExecutor() *ops.NativeExecutor {
	executor := &ops.NativeExecutor{Secrets: app.tryGetSecretStore()}
	executor.PushFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		method, destination, err := transferOne(ctx, host, executor.Secrets, transferOptions{
			direction: "push", localPath: options.Src, remotePath: options.Dest,
			overwrite: options.Overwrite, method: options.Method, connectTimeout: options.ConnectTimeout,
		})
		return ops.NewTransferResult(host, method, destination, err, time.Since(start))
	}
	executor.PullFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		method, destination, err := transferOne(ctx, host, executor.Secrets, transferOptions{
			direction: "pull", localPath: options.Dest, remotePath: options.Src,
			overwrite: options.Overwrite, method: options.Method, connectTimeout: options.ConnectTimeout,
		})
		return ops.NewTransferResult(host, method, destination, err, time.Since(start))
	}
	return executor
}

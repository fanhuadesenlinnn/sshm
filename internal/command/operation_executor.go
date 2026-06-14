package command

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

func (app *App) operationExecutor() *ops.NativeExecutor {
	executor := &ops.NativeExecutor{Secrets: app.tryGetSecretStore()}
	executor.PushFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		var diff bytes.Buffer
		var diffWriter io.Writer
		if options.Diff {
			diffWriter = &diff
		}
		method, destination, changed, err := transferOne(ctx, host, executor.Secrets, transferOptions{
			direction: "push", localPath: options.Src, remotePath: options.Dest,
			overwrite: options.Overwrite, backup: options.Backup, validateChecksum: options.ValidateChecksum,
			destinationExact: options.DestinationExact, check: options.Check, diffWriter: diffWriter,
			method: options.Method, connectTimeout: options.ConnectTimeout,
		})
		result := ops.NewTransferResultWithChange(host, method, destination, changed, err, time.Since(start))
		result.WouldChange = options.Check && changed
		result.Output = diff.String()
		return result
	}
	executor.PullFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		method, destination, changed, err := transferOne(ctx, host, executor.Secrets, transferOptions{
			direction: "pull", localPath: options.Dest, remotePath: options.Src,
			overwrite: options.Overwrite, backup: options.Backup, validateChecksum: options.ValidateChecksum,
			destinationExact: options.DestinationExact, check: options.Check, method: options.Method, connectTimeout: options.ConnectTimeout,
		})
		result := ops.NewTransferResultWithChange(host, method, destination, changed, err, time.Since(start))
		result.WouldChange = options.Check && changed
		return result
	}
	return executor
}

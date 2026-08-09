package command

import (
	"context"
	"io"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
)

func (app *App) operationExecutor() *ops.NativeExecutor {
	executor := &ops.NativeExecutor{Secrets: app.tryGetSecretStore()}
	executor.PushFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		client, sftpClient, release, err := executor.ReusableSession(ctx, host, options.ConnectTimeout)
		if err != nil {
			return ops.NewTransferResult(host, "sftp", "", err, time.Since(start))
		}
		invalidateSession := true
		defer func() { release(invalidateSession) }()
		diff := newBoundedCapture(8 << 20)
		var diffWriter io.Writer
		if options.Diff {
			diffWriter = diff
		}
		method, destination, changed, err := transferWithClients(ctx, host, executor.Secrets, transferOptions{
			direction: "push", localPath: options.Src, remotePath: options.Dest,
			overwrite: options.Overwrite, backup: options.Backup, validateChecksum: options.ValidateChecksum,
			destinationExact: options.DestinationExact, check: options.Check, diffWriter: diffWriter,
			method: options.Method, connectTimeout: options.ConnectTimeout,
		}, client, sftpClient, nil)
		invalidateSession = err != nil || ctx.Err() != nil
		result := ops.NewTransferResultWithChange(host, method, destination, changed, err, time.Since(start))
		result.WouldChange = options.Check && changed
		result.Output = diff.String()
		return result
	}
	executor.PullFunc = func(ctx context.Context, host config.Host, options ops.TransferOptions) ops.Result {
		start := time.Now()
		client, sftpClient, release, err := executor.ReusableSession(ctx, host, options.ConnectTimeout)
		if err != nil {
			return ops.NewTransferResult(host, "sftp", "", err, time.Since(start))
		}
		invalidateSession := true
		defer func() { release(invalidateSession) }()
		method, destination, changed, err := transferWithClients(ctx, host, executor.Secrets, transferOptions{
			direction: "pull", localPath: options.Dest, remotePath: options.Src,
			overwrite: options.Overwrite, backup: options.Backup, validateChecksum: options.ValidateChecksum,
			destinationExact: options.DestinationExact, check: options.Check, method: options.Method, connectTimeout: options.ConnectTimeout,
		}, client, sftpClient, nil)
		invalidateSession = err != nil || ctx.Err() != nil
		result := ops.NewTransferResultWithChange(host, method, destination, changed, err, time.Since(start))
		result.WouldChange = options.Check && changed
		return result
	}
	return executor
}

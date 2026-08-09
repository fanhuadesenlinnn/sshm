package command

import (
	"context"
	"fmt"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdPing(args []string) error {
	yes, args := removeFlag(args, "--yes")
	quiet, args := removeFlag(args, "--quiet")
	if len(args) > 1 {
		return fmt.Errorf("用法: sshmd ping [别名|ID] [--yes] [--quiet]")
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return err
	}
	connectTimeout := doc.Defaults.Batch.ConnectTimeout.Duration

	if len(args) > 0 {
		// Ping specific host
		h, _, _, err := app.findHost(args[0])
		if err != nil {
			return err
		}
		if err := app.unlockVaultForHosts([]config.Host{*h}); err != nil {
			return &ExitError{Code: 4, Err: err}
		}
		fs := app.getSecretStoreForHost(h)
		start := time.Now()
		baseCtx, stop := signalContext()
		defer stop()
		ctx, cancel := context.WithTimeout(baseCtx, connectTimeout)
		output, pingErr := sshx.CheckPingContext(ctx, *h, fs)
		cancel()
		stage := operation.StageOf(pingErr, operation.StageExecute)
		result := newOperationResult(*h, output, pingErr, stage, fmt.Sprintf("sshmd ping %s", h.Alias), time.Since(start))
		if pingErr == nil {
			ui.PrintSuccess("%s (%s@%s:%d) 连接成功", h.Alias, h.User, h.Host, h.Port)
		} else {
			printOperationFailure(result)
			if logErr := writeOperationLog("ping", "连接测试", []operation.Result{result}); logErr != nil {
				return logErr
			}
			code := 1
			if operation.IsConnectionFailure(result.Stage) {
				code = 2
			}
			return &ExitError{Code: code, Err: fmt.Errorf("主机 %s 连接失败: %w", h.Alias, pingErr)}
		}
	} else {
		// Ping all hosts
		if len(hf.Hosts) == 0 {
			ui.PrintWarn("暂无主机")
			return nil
		}
		if !yes {
			if !ui.IsTerminal() {
				return fmt.Errorf("多主机 ping 需要确认；非交互环境请使用 --yes")
			}
			fmt.Printf("即将测试 %d 台主机:\n", len(hf.Hosts))
			for _, host := range hf.Hosts {
				fmt.Printf("  - %s (%s)\n", host.Alias, host.Host)
			}
			if !ui.ReadYesNo("确认执行? [y/N]: ") {
				ui.PrintWarn("已取消")
				return nil
			}
		}

		fmt.Println()
		ui.PrintHeader("测试所有主机连接")
		fmt.Println()

		if err := app.unlockVaultForHosts(hf.Hosts); err != nil {
			return &ExitError{Code: 4, Err: err}
		}
		// Pre-load secret store if any host has password or a managed key.
		fs := app.getSecretStoreForPing(hf.Hosts)
		ctx, stop := signalContext()
		defer stop()
		runResult, runErr := runPingBatch(ctx, hf.Hosts, doc.Defaults.Batch.Parallel, connectTimeout, fs, sshx.CheckPingContext, func(result operation.Result) {
			fmt.Printf("  %-18s ", result.Host.Alias)
			if result.Err == nil {
				fmt.Println(ui.Success("ok"))
				return
			}
			fmt.Println(ui.ErrorMsg("fail"))
			if !quiet {
				printOperationFailure(result)
			}
		})
		if runErr != nil {
			return runErr
		}
		results := pingOperationResults(runResult)
		failed := runResult.Summary.Failed + runResult.Summary.Unreachable
		fmt.Println()
		fmt.Printf("连接测试完成：成功 %d，失败 %d，跳过 %d\n", runResult.Summary.OK, failed, runResult.Summary.Skipped)
		if err := writeOperationLog("ping-batch", "连接测试", results); err != nil {
			return err
		}
		code := batch.ExitCode(runResult)
		if code != 0 {
			if quiet {
				for _, result := range results {
					if result.Err != nil {
						printOperationFailure(result)
					}
				}
			}
			return &ExitError{Code: code, Err: fmt.Errorf("连接测试未完全成功：失败 %d，跳过 %d", failed, runResult.Summary.Skipped)}
		}
	}

	return nil
}

type pingCheckFunc func(context.Context, config.Host, *secret.FileStore) (string, error)

func runPingBatch(
	ctx context.Context,
	hosts []config.Host,
	parallel int,
	timeout time.Duration,
	store *secret.FileStore,
	check pingCheckFunc,
	progress func(operation.Result),
) (batch.RunResult, error) {
	runner := batch.Runner{Options: batch.Options{Parallel: parallel}}
	if progress != nil {
		runner.Progress = func(_, _ int, item batch.Result) {
			if result, ok := item.Value.(operation.Result); ok {
				progress(result)
			}
		}
	}
	return runner.Run(ctx, hosts, func(ctx context.Context, host config.Host) batch.Result {
		started := time.Now()
		taskCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		output, err := check(taskCtx, host, store)
		stage := operation.StageOf(err, operation.StageExecute)
		result := newOperationResult(host, output, err, stage, fmt.Sprintf("sshmd ping %s", host.Alias), time.Since(started))
		status := batch.StatusOK
		if err != nil {
			status = batch.StatusFailed
			if operation.IsConnectionFailure(stage) {
				status = batch.StatusUnreachable
			}
		}
		return batch.Result{Status: status, Err: err, Detail: output, Value: result}
	})
}

func pingOperationResults(runResult batch.RunResult) []operation.Result {
	results := make([]operation.Result, 0, len(runResult.Results))
	for _, item := range runResult.Results {
		if result, ok := item.Value.(operation.Result); ok {
			results = append(results, result)
		} else if item.Status == batch.StatusSkipped {
			results = append(results, skippedOperationResult(item.Host, item.SkippedReason))
		}
	}
	return results
}

// getSecretStoreForHost returns a secret store if the host has a password.
func (app *App) getSecretStoreForHost(h *config.Host) *secret.FileStore {
	if h.PasswordRef == "" {
		if _, managed := config.ManagedKeyName(h.Identity); !managed {
			return nil
		}
	}
	return app.tryGetSecretStore()
}

// getSecretStoreForPing returns a secret store if any host needs one.
func (app *App) getSecretStoreForPing(hosts []config.Host) *secret.FileStore {
	for _, h := range hosts {
		if h.PasswordRef != "" {
			return app.tryGetSecretStore()
		}
		if _, managed := config.ManagedKeyName(h.Identity); managed {
			return app.tryGetSecretStore()
		}
	}
	return nil
}

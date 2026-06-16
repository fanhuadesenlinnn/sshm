package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdExec(args []string) error {
	yes, quiet, args := parseOperationFlags(args)
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm exec [--yes] [--quiet] <别名|ID> <命令>")
	}

	h, _, _, err := app.Store.FindHost(args[0])
	if err != nil {
		return err
	}

	command := strings.Join(args[1:], " ")
	if !yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("远程命令执行需要确认；非交互环境请使用 --yes")
		}
		fmt.Printf("即将在 %s (%s) 执行: %s\n", h.Alias, h.Host, command)
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	opResult := app.operationExecutor().Exec(context.Background(), *h, ops.ExecOptions{Command: command})
	err = opResult.Err
	result := newOperationResult(*h, opResult.Output, err, operation.StageExecute,
		fmt.Sprintf("sshm exec --yes %s %q", h.Alias, command), opResult.Duration)
	if err != nil {
		printOperationFailure(result)
	}
	if !quiet {
		fmt.Print(opResult.Output)
	} else {
		if logErr := writeOperationLog("exec", command, []operation.Result{result}); logErr != nil {
			return logErr
		}
	}
	return err
}

func (app *App) cmdExecTag(args []string) error {
	options, positionals, err := parseBatchCLIOptions(args)
	if err != nil {
		return err
	}
	if len(positionals) < 2 {
		return fmt.Errorf("用法: sshm exec-tag <标签> <命令>")
	}

	tagName := positionals[0]
	command := strings.Join(positionals[1:], " ")

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	var hosts []config.Host
	for _, h := range hf.Hosts {
		if tagName == "all" || h.HasTag(tagName) {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return fmt.Errorf("没有匹配标签 %q 的主机", tagName)
	}
	return app.executeBatch(hosts, command, options)
}

func (app *App) executeBatch(hosts []config.Host, command string, options batchCLIOptions) error {
	if !options.Yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("批量远程命令需要确认；非交互环境请使用 --yes")
		}
		fmt.Printf("\n即将在 %d 台主机上执行: %s\n\n", len(hosts), command)
		for _, h := range hosts {
			fmt.Printf("  - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
		}
		fmt.Println()
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	if err := app.unlockVaultForHosts(hosts); err != nil {
		return &ExitError{Code: 4, Err: err}
	}
	batchOptions, timeout, connectTimeout, logDefaults, err := app.resolveBatchOptions(options, false)
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}
	executor := app.operationExecutor()
	ctx, cancel := signalContext()
	defer cancel()
	runner := batch.Runner{
		Options: batchOptions,
		Progress: func(done, total int, _ batch.Result) {
			ui.RefreshLine("  [%d/%d] 执行中...", done, total)
		},
	}
	runResult, err := runner.Run(ctx, hosts, func(ctx context.Context, host config.Host) batch.Result {
		taskCtx, taskCancel := context.WithTimeout(ctx, timeout)
		defer taskCancel()
		opResult := executor.Exec(taskCtx, host, ops.ExecOptions{Command: command, ConnectTimeout: connectTimeout})
		return batch.Result{Status: batchStatus(opResult), Err: opResult.Err, Detail: opResult.Output, Value: opResult}
	})
	ui.EndProgress()
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}

	logResults := make([]operation.Result, 0, len(runResult.Results))
	for _, result := range runResult.Results {
		if !options.Quiet {
			fmt.Println()
			ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", result.Host.Alias, result.Host.User, result.Host.Host))
		}
		if result.Status == batch.StatusSkipped {
			ui.PrintWarn("%s: skipped (%s)", result.Host.Alias, result.SkippedReason)
			logResults = append(logResults, skippedOperationResult(result.Host, result.SkippedReason))
			continue
		}
		opResult := result.Value.(ops.Result)
		if !options.Quiet {
			fmt.Print(opResult.Output)
		}
		logResult := newOperationResult(result.Host, opResult.Output, opResult.Err, operation.StageExecute,
			fmt.Sprintf("sshm exec --yes %s %q", result.Host.Alias, command), opResult.Duration)
		logResults = append(logResults, logResult)
		if opResult.Err != nil {
			printOperationFailure(logResult)
		}
	}
	fmt.Println()
	printBatchSummary(runResult.Summary)
	if logDefaults.Enabled && !options.NoLog {
		if err := writeOperationLog("exec-batch", command, logResults); err != nil {
			return err
		}
	}
	return exitErrorForBatch(runResult)
}

func printBatchSummary(summary batch.Summary) {
	fmt.Println("summary:")
	fmt.Printf("  ok: %d\n", summary.OK)
	fmt.Printf("  changed: %d\n", summary.Changed)
	fmt.Printf("  would-change: %d\n", summary.WouldChange)
	fmt.Printf("  failed: %d\n", summary.Failed)
	fmt.Printf("  unreachable: %d\n", summary.Unreachable)
	fmt.Printf("  skipped: %d\n", summary.Skipped)
}

func parseOperationFlags(args []string) (yes, quiet bool, rest []string) {
	for len(args) > 0 {
		switch args[0] {
		case "--yes":
			yes = true
		case "--quiet":
			quiet = true
		default:
			return yes, quiet, args
		}
		args = args[1:]
	}
	return yes, quiet, nil
}

// tryGetSecretStore attempts to create a secret store, prompting for master password.
// Allows up to 3 retries before giving up.
// Returns nil if no vault exists, stdin is not a terminal, or authentication fails.
func (app *App) tryGetSecretStore() *secret.FileStore {
	if app.secretStore != nil {
		return app.secretStore
	}
	exists, err := secret.VaultExists(app.ConfigPath)
	if err != nil || !exists {
		return nil
	}
	// Only prompt if stdin is a terminal
	if !ui.IsTerminal() {
		return nil
	}

	for attempt := 1; attempt <= 3; attempt++ {
		pass, err := ui.ReadPassword("请输入 sshm 主密码: ")
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.Warn("读取主密码失败，跳过密码认证"))
			return nil
		}
		fs := secret.NewFileStore(app.ConfigPath, pass)
		err = fs.VerifyPassphrase()
		if err == nil {
			app.secretStore = fs
			return fs
		}
		if !errors.Is(err, secret.ErrIncorrectPassphrase) {
			fmt.Fprintln(os.Stderr, ui.Warn("无法读取 vault: %v", err))
			return nil
		}
		if attempt < 3 {
			fmt.Fprintf(os.Stderr, "%s\n", ui.Warn("主密码错误，请重试 (%d/3)", attempt))
		}
	}

	fmt.Fprintln(os.Stderr, ui.Warn("主密码错误，跳过密码认证"))
	return nil
}

package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdExec(args []string) error {
	yes, quiet, noLog, aliasOrID, command, err := parseExecArgs(args)
	if err != nil {
		return err
	}

	h, _, _, err := app.findHost(aliasOrID)
	if err != nil {
		return err
	}

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

	executor := app.operationExecutor()
	defer executor.CloseSessions()
	var stdout, stderr io.Writer
	if !quiet {
		stdout = os.Stdout
		stderr = os.Stderr
	}
	opResult := executor.Exec(context.Background(), *h, ops.ExecOptions{Command: command, Stdout: stdout, Stderr: stderr})
	err = opResult.Err
	result := newOperationResult(*h, opResult.Output, err, operation.StageExecute,
		fmt.Sprintf("sshm exec --yes %s %s", shellquote.Single(h.Alias), shellquote.Single(command)), opResult.Duration)
	if err != nil {
		printOperationFailure(result)
	}
	if !noLog {
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
		return fmt.Errorf("用法: sshm exec-tag [批量选项] <标签> [--] <命令>")
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
		return fmt.Errorf("没有匹配标签 %q 的主机；使用 sshm tag list 查看标签，或 sshm tag add %s <主机> 绑定主机", tagName, tagName)
	}
	hosts, err = deploy.ApplyExcludes(hosts, hf.Hosts, options.Exclude, options.ExcludeTags)
	if err != nil {
		return err
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
	defer executor.CloseSessions()
	ctx, cancel := signalContext()
	defer cancel()
	logResults := make([]operation.Result, 0, len(hosts))
	runner := batch.Runner{
		Options: batchOptions,
		Progress: func(done, total int, item batch.Result) {
			// 每台主机完成即打印其输出，避免整批结束后才看到内容。
			ui.EndProgress()
			if !options.Quiet {
				fmt.Println()
				ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", item.Host.Alias, item.Host.User, item.Host.Host))
			}
			if item.Status == batch.StatusSkipped {
				ui.PrintWarn("%s: skipped (%s)", item.Host.Alias, item.SkippedReason)
				logResults = append(logResults, skippedOperationResult(item.Host, item.SkippedReason))
				return
			}
			opResult := item.Value.(ops.Result)
			if !options.Quiet {
				fmt.Print(opResult.Output)
			}
			logResult := newOperationResult(item.Host, opResult.Output, opResult.Err, operation.StageExecute,
				fmt.Sprintf("sshm exec --yes %s %s", shellquote.Single(item.Host.Alias), shellquote.Single(command)), opResult.Duration)
			logResults = append(logResults, logResult)
			if opResult.Err != nil {
				printOperationFailure(logResult)
			}
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

func parseOperationFlags(args []string) (yes, quiet, noLog bool, rest []string) {
	for len(args) > 0 {
		switch args[0] {
		case "--yes":
			yes = true
		case "--quiet":
			quiet = true
		case "--no-log":
			noLog = true
		default:
			return yes, quiet, noLog, args
		}
		args = args[1:]
	}
	return yes, quiet, noLog, nil
}

func parseExecArgs(args []string) (yes, quiet, noLog bool, aliasOrID, command string, err error) {
	yes, quiet, noLog, args = parseOperationFlags(args)
	if len(args) < 2 {
		return false, false, false, "", "", fmt.Errorf("用法: sshm exec [--yes] [--quiet] [--no-log] <别名|ID> [--] <命令>")
	}
	aliasOrID = args[0]
	commandArgs := append([]string(nil), args[1:]...)
	if len(commandArgs) > 0 && commandArgs[0] == "--" {
		commandArgs = commandArgs[1:]
		if len(commandArgs) == 0 {
			return false, false, false, "", "", fmt.Errorf("用法: sshm exec [--yes] [--quiet] [--no-log] <别名|ID> [--] <命令>")
		}
		return yes, quiet, noLog, aliasOrID, strings.Join(commandArgs, " "), nil
	}
	for len(commandArgs) > 0 {
		last := commandArgs[len(commandArgs)-1]
		switch last {
		case "--yes":
			yes = true
		case "--quiet":
			quiet = true
		case "--no-log":
			noLog = true
		default:
			return yes, quiet, noLog, aliasOrID, strings.Join(commandArgs, " "), nil
		}
		commandArgs = commandArgs[:len(commandArgs)-1]
	}
	return false, false, false, "", "", fmt.Errorf("用法: sshm exec [--yes] [--quiet] [--no-log] <别名|ID> [--] <命令>")
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
	fs, ok, err := app.secretStoreFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.Warn("%v", err))
		return nil
	}
	if ok {
		return fs
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

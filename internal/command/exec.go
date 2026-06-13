package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
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

	fs := app.tryGetSecretStore()
	start := time.Now()
	output, err := sshx.ExecCommand(*h, fs, command)
	result := newOperationResult(*h, output, err, operation.StageExecute,
		fmt.Sprintf("sshm exec --yes %s %q", h.Alias, command), time.Since(start))
	if err != nil {
		printOperationFailure(result)
	}
	if !quiet {
		fmt.Print(output)
	} else {
		if logErr := writeOperationLog("exec", command, []operation.Result{result}); logErr != nil {
			return logErr
		}
	}
	return err
}

func (app *App) cmdExecTag(args []string) error {
	yes, quiet, args := parseOperationFlags(args)
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm exec-tag <标签> <命令>")
	}

	tagFilter := args[0]
	command := strings.Join(args[1:], " ")

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	var hosts []config.Host
	filters := config.ParseTags(tagFilter)
	for _, h := range hf.Hosts {
		if h.MatchTags(filters) {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return fmt.Errorf("没有匹配标签 %q 的主机", tagFilter)
	}
	return app.executeBatch(hosts, command, yes, quiet)
}

func (app *App) cmdExecAll(args []string) error {
	yes, quiet, args := parseOperationFlags(args)
	if len(args) < 1 {
		return fmt.Errorf("用法: sshm exec-all <命令>")
	}

	command := strings.Join(args, " ")

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	if len(hf.Hosts) == 0 {
		return fmt.Errorf("暂无主机可执行")
	}
	return app.executeBatch(hf.Hosts, command, yes, quiet)
}

type batchExecResult struct {
	host     config.Host
	output   string
	err      error
	duration time.Duration
}

func (app *App) executeBatch(hosts []config.Host, command string, yes, quiet bool) error {
	// List affected hosts and ask for confirmation.
	if !yes {
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

	fs := app.tryGetSecretStore()
	results := make([]batchExecResult, len(hosts))
	progress := make(chan int, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(4, len(hosts))

	// Print progress as results come in.
	go func() {
		done := 0
		for range progress {
			done++
			fmt.Fprintf(os.Stderr, "\r  [%d/%d] 执行中...", done, len(hosts))
		}
		fmt.Fprintln(os.Stderr)
	}()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				output, err := sshx.ExecCommandContext(ctx, hosts[i], fs, command)
				cancel()
				results[i] = batchExecResult{host: hosts[i], output: output, err: err, duration: time.Since(start)}
				progress <- i
			}
		}()
	}
	for i := range hosts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(progress)

	failed := 0
	var failedHosts []string
	logResults := make([]operation.Result, 0, len(results))
	for _, result := range results {
		if !quiet {
			fmt.Println()
			ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", result.host.Alias, result.host.User, result.host.Host))
		}
		if result.err != nil {
			failed++
			failedHosts = append(failedHosts, result.host.Alias)
		}
		if !quiet {
			fmt.Print(result.output)
		}
		opResult := newOperationResult(result.host, result.output, result.err, operation.StageExecute,
			fmt.Sprintf("sshm exec --yes %s %q", result.host.Alias, command), result.duration)
		logResults = append(logResults, opResult)
		if result.err != nil {
			printOperationFailure(opResult)
		}
	}
	fmt.Println()
	fmt.Printf("批量执行完成：成功 %d，失败 %d\n", len(results)-failed, failed)
	if err := writeOperationLog("exec-batch", command, logResults); err != nil {
		return err
	}
	if failed > 0 {
		fmt.Printf("失败主机: %s\n", strings.Join(failedHosts, ", "))
		if failed == len(hosts) {
			return fmt.Errorf("所有 %d 台主机执行失败", failed)
		}
		return fmt.Errorf("有 %d 台主机执行失败", failed)
	}
	return nil
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

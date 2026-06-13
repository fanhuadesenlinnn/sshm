package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdExec(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm exec <别名|ID> <命令>")
	}

	h, _, _, err := app.Store.FindHost(args[0])
	if err != nil {
		return err
	}

	command := strings.Join(args[1:], " ")

	fs := app.tryGetSecretStore()
	output, err := sshx.ExecCommand(*h, fs, command)
	if err != nil {
		ui.PrintError("执行失败: %v", err)
	}
	fmt.Print(output)
	return err
}

func (app *App) cmdExecTag(args []string) error {
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
	return app.executeBatch(hosts, command)
}

func (app *App) cmdExecAll(args []string) error {
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
	return app.executeBatch(hf.Hosts, command)
}

type batchExecResult struct {
	host   config.Host
	output string
	err    error
}

func (app *App) executeBatch(hosts []config.Host, command string) error {
	// List affected hosts and ask for confirmation.
	fmt.Println()
	fmt.Printf("即将在 %d 台主机上执行: %s\n", len(hosts), command)
	fmt.Println()
	for _, h := range hosts {
		fmt.Printf("  - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
	}
	fmt.Println()
	if !ui.ReadYesNo("确认执行? [y/N]: ") {
		ui.PrintWarn("已取消")
		return nil
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
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				output, err := sshx.ExecCommandContext(ctx, hosts[i], fs, command)
				cancel()
				results[i] = batchExecResult{host: hosts[i], output: output, err: err}
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
	for _, result := range results {
		fmt.Println()
		ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", result.host.Alias, result.host.User, result.host.Host))
		if result.err != nil {
			failed++
			failedHosts = append(failedHosts, result.host.Alias)
			ui.PrintError("执行失败: %v", result.err)
		}
		fmt.Print(result.output)
	}
	fmt.Println()
	fmt.Printf("批量执行完成：成功 %d，失败 %d\n", len(results)-failed, failed)
	if failed > 0 {
		fmt.Printf("失败主机: %s\n", strings.Join(failedHosts, ", "))
		if failed == len(hosts) {
			return fmt.Errorf("所有 %d 台主机执行失败", failed)
		}
		return fmt.Errorf("有 %d 台主机执行失败", failed)
	}
	return nil
}

// tryGetSecretStore attempts to create a secret store, prompting for master password.
// Allows up to 3 retries before giving up.
// Returns nil if no secrets file exists, stdin is not a terminal, or authentication fails.
func (app *App) tryGetSecretStore() *secret.FileStore {
	if app.secretStore != nil {
		return app.secretStore
	}
	// If no secrets file exists, return nil
	if _, err := os.Stat(app.SecretPath); os.IsNotExist(err) {
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
		fs := secret.NewFileStore(app.SecretPath, pass)
		err = fs.VerifyPassphrase()
		if err == nil {
			app.secretStore = fs
			if migrateErr := app.migratePasswordRefs(fs); migrateErr != nil {
				fmt.Fprintln(os.Stderr, ui.Warn("迁移旧密码引用失败: %v", migrateErr))
			}
			return fs
		}
		if !errors.Is(err, secret.ErrIncorrectPassphrase) {
			fmt.Fprintln(os.Stderr, ui.Warn("无法读取 secrets 文件: %v", err))
			return nil
		}
		if attempt < 3 {
			fmt.Fprintf(os.Stderr, "%s\n", ui.Warn("主密码错误，请重试 (%d/3)", attempt))
		}
	}

	fmt.Fprintln(os.Stderr, ui.Warn("主密码错误，跳过密码认证"))
	return nil
}

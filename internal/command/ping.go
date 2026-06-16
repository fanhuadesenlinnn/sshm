package command

import (
	"context"
	"fmt"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdPing(args []string) error {
	yes, args := removeFlag(args, "--yes")
	quiet, args := removeFlag(args, "--quiet")
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		// Ping specific host
		h, _, _, err := app.Store.FindHost(args[0])
		if err != nil {
			return err
		}
		fs := app.getSecretStoreForHost(h)
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		output, pingErr := sshx.CheckPingContext(ctx, *h, fs)
		cancel()
		result := newOperationResult(*h, output, pingErr, operation.StageExecute, fmt.Sprintf("sshm ping %s", h.Alias), time.Since(start))
		if pingErr == nil {
			ui.PrintSuccess("%s (%s@%s:%d) 连接成功", h.Alias, h.User, h.Host, h.Port)
		} else {
			printOperationFailure(result)
			return fmt.Errorf("主机 %s 连接失败: %w", h.Alias, pingErr)
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

		// Pre-load secret store if any host has password
		fs := app.getSecretStoreForPing(hf.Hosts)

		failed := 0
		results := make([]operation.Result, 0, len(hf.Hosts))
		for _, h := range hf.Hosts {
			fmt.Printf("  %-18s ", h.Alias)
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			output, pingErr := sshx.CheckPingContext(ctx, h, fs)
			cancel()
			result := newOperationResult(h, output, pingErr, operation.StageExecute, fmt.Sprintf("sshm ping %s", h.Alias), time.Since(start))
			results = append(results, result)
			if pingErr == nil {
				fmt.Println(ui.Success("ok"))
			} else {
				failed++
				fmt.Println(ui.ErrorMsg("fail"))
				if !quiet {
					printOperationFailure(result)
				}
			}
		}
		fmt.Println()
		fmt.Printf("连接测试完成：成功 %d，失败 %d\n", len(hf.Hosts)-failed, failed)
		if err := writeOperationLog("ping-batch", "连接测试", results); err != nil {
			return err
		}
		if failed > 0 {
			if quiet {
				for _, result := range results {
					if result.Err != nil {
						printOperationFailure(result)
					}
				}
			}
			code := 2
			for _, result := range results {
				if !operation.IsConnectionFailure(result.Stage) {
					if result.Err != nil {
						code = 1
					}
				}
			}
			return &ExitError{Code: code, Err: fmt.Errorf("有 %d 台主机连接失败", failed)}
		}
	}

	return nil
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

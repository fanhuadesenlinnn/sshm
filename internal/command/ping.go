package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdPing(args []string) error {
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
		ok, msg := sshx.CheckPing(*h, fs)
		if ok {
			ui.PrintSuccess("%s (%s@%s:%d) 连接成功", h.Alias, h.User, h.Host, h.Port)
		} else {
			ui.PrintError("%s (%s@%s:%d) 连接失败: %s", h.Alias, h.User, h.Host, h.Port, msg)
			return fmt.Errorf("主机 %s 连接失败", h.Alias)
		}
	} else {
		// Ping all hosts
		if len(hf.Hosts) == 0 {
			ui.PrintWarn("暂无主机")
			return nil
		}

		fmt.Println()
		ui.PrintHeader("测试所有主机连接")
		fmt.Println()

		// Pre-load secret store if any host has password
		fs := app.getSecretStoreForPing(hf.Hosts)

		failed := 0
		for _, h := range hf.Hosts {
			fmt.Printf("  %-18s ", h.Alias)
			ok, msg := sshx.CheckPing(h, fs)
			if ok {
				fmt.Println(ui.Success("ok"))
			} else {
				failed++
				fmt.Println(ui.ErrorMsg("fail - %s", msg))
			}
		}
		fmt.Println()
		fmt.Printf("连接测试完成：成功 %d，失败 %d\n", len(hf.Hosts)-failed, failed)
		if failed > 0 {
			return fmt.Errorf("有 %d 台主机连接失败", failed)
		}
	}

	return nil
}

// getSecretStoreForHost returns a secret store if the host has a password.
func (app *App) getSecretStoreForHost(h *config.Host) *secret.FileStore {
	if h.PasswordRef == "" {
		return nil
	}
	return app.tryGetSecretStore()
}

// getSecretStoreForPing returns a secret store if any host has a password.
func (app *App) getSecretStoreForPing(hosts []config.Host) *secret.FileStore {
	for _, h := range hosts {
		if h.PasswordRef != "" {
			return app.tryGetSecretStore()
		}
	}
	return nil
}

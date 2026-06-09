package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/secret"
	"github.com/sshm/sshm/internal/sshx"
	"github.com/sshm/sshm/internal/ui"
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

		for _, h := range hf.Hosts {
			fmt.Printf("  %-18s ", h.Alias)
			ok, msg := sshx.CheckPing(h, fs)
			if ok {
				fmt.Println(ui.Success("ok"))
			} else {
				fmt.Println(ui.ErrorMsg("fail - %s", msg))
			}
		}
		fmt.Println()
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

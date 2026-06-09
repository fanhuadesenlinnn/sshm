package command

import (
	"fmt"

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
		ok, msg := sshx.CheckPing(*h, nil)
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

		for _, h := range hf.Hosts {
			fmt.Printf("  %-18s ", h.Alias)
			ok, msg := sshx.CheckPing(h, nil)
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

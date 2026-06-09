package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdCopy(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入要复制的主机别名或ID: ")
	if err != nil {
		return err
	}

	connStr := fmt.Sprintf("ssh -p %d %s@%s", h.Port, h.User, h.Host)
	fmt.Println()
	ui.PrintHeader("连接信息已复制到屏幕")
	fmt.Println()
	fmt.Printf("  %s\n", connStr)
	fmt.Println()
	fmt.Println("  提示：可直接使用 sshm " + h.Alias + " 连接")
	fmt.Println()
	return nil
}

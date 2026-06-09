package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdDelete(args []string) error {
	h, idx, _, err := app.resolveHost(args, "请输入要删除的主机别名或ID: ")
	if err != nil {
		return err
	}

	ui.RenderHostDetail(*h, idx)

	confirm := ui.ReadLine(fmt.Sprintf("确认删除 %s? [y/N]: ", h.Alias))
	if confirm != "y" && confirm != "Y" && confirm != "yes" {
		ui.PrintWarn("已取消")
		return nil
	}

	if err := app.Store.Remove(idx); err != nil {
		return err
	}

	ui.PrintSuccess("已删除主机：%s", h.Alias)
	return nil
}

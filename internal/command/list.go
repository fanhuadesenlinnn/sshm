package command

import (
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdList(args []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	if len(hf.Hosts) == 0 {
		ui.PrintWarn("暂无主机，使用 --add 添加")
		return nil
	}

	ui.PrintHeader("主机列表")
	ui.RenderHostsTable(hf.Hosts)
	return nil
}

package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdPick(_ []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	if len(hf.Hosts) == 0 {
		return fmt.Errorf("暂无主机，请先使用 sshm add 添加")
	}
	alias, ok := ui.PickHost(hf.Hosts)
	if !ok {
		return nil
	}
	return app.cmdConnect([]string{alias})
}

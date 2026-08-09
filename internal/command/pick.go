package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdPick(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("用法: sshmd find-con")
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	if len(hf.Hosts) == 0 {
		return fmt.Errorf("暂无主机，请先使用 sshmd add 添加")
	}
	alias, ok := ui.PickHost(hf.Hosts)
	if !ok {
		return nil
	}
	return app.cmdConnect([]string{alias})
}

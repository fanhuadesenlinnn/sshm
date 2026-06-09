package command

import (
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdShow(args []string) error {
	h, idx, _, err := app.resolveHost(args, "请输入要查看的主机别名或ID: ")
	if err != nil {
		return err
	}

	ui.RenderHostDetail(*h, idx)
	return nil
}

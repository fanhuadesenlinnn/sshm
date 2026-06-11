package command

import (
	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdGroup(args []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	groups := map[string][]config.Host{}
	for _, h := range hf.Hosts {
		if len(args) > 0 && h.Group != args[0] {
			continue
		}
		groups[h.Group] = append(groups[h.Group], h)
	}

	ui.RenderGroupList(groups)
	return nil
}

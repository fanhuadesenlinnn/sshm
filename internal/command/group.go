package command

import (
	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdTag(args []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	tagFilter := ""
	if len(args) > 0 {
		tagFilter = args[0]
	}

	tags := map[string][]config.Host{}
	for _, h := range hf.Hosts {
		for _, t := range h.Tags {
			if tagFilter != "" && t != tagFilter {
				continue
			}
			tags[t] = append(tags[t], h)
		}
	}

	ui.RenderTagList(tags)
	return nil
}

package command

import (
	"strings"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdSearch(args []string) error {
	keyword := ""
	if len(args) > 0 {
		keyword = args[0]
	} else {
		keyword = ui.ReadLine("请输入搜索关键词: ")
	}
	if keyword == "" {
		return nil
	}

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	keyword = strings.ToLower(keyword)
	var results []config.Host
	for _, h := range hf.Hosts {
		if matchHost(h, keyword) {
			results = append(results, h)
		}
	}

	ui.RenderSearchResults(results, keyword)
	return nil
}

func matchHost(h config.Host, keyword string) bool {
	if strings.Contains(strings.ToLower(h.Alias), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(h.Host), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(h.User), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(h.Note), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(h.Group), keyword) {
		return true
	}
	for _, t := range h.Tags {
		if strings.Contains(strings.ToLower(t), keyword) {
			return true
		}
	}
	return false
}

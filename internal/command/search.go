package command

import (
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdSearch(args []string) error {
	query := ""
	if len(args) > 0 {
		query = strings.Join(args, " ")
	} else {
		query = ui.ReadLine("请输入搜索关键词: ")
	}
	if query == "" {
		return nil
	}

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	terms := strings.Fields(strings.ToLower(query))
	var results []config.Host
	var indices []int
	for i, h := range hf.Hosts {
		if matchHostTerms(h, terms) {
			results = append(results, h)
			indices = append(indices, i)
		}
	}

	ui.RenderSearchResults(results, indices, query)
	return nil
}

func matchHostTerms(h config.Host, terms []string) bool {
	for _, term := range terms {
		switch {
		case strings.HasPrefix(term, "group:"):
			if !strings.Contains(strings.ToLower(h.Group), strings.TrimPrefix(term, "group:")) {
				return false
			}
		case strings.HasPrefix(term, "tag:"):
			if !matchTag(h.Tags, strings.TrimPrefix(term, "tag:")) {
				return false
			}
		default:
			if !matchHost(h, term) {
				return false
			}
		}
	}
	return true
}

func matchTag(tags []string, keyword string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), keyword) {
			return true
		}
	}
	return false
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
	return matchTag(h.Tags, keyword)
}

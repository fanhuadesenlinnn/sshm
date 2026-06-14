package command

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

func (app *App) cmdPin(args []string, pinned bool) error {
	h, idx, _, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}
	if h.Pinned == pinned {
		if pinned {
			ui.PrintSuccess("%s 已在收藏中", h.Alias)
		} else {
			ui.PrintSuccess("%s 未被收藏", h.Alias)
		}
		return nil
	}
	h.Pinned = pinned
	if err := app.Store.Update(idx, *h); err != nil {
		return fmt.Errorf("更新收藏状态失败: %w", err)
	}
	if pinned {
		ui.PrintSuccess("已收藏主机：%s", h.Alias)
	} else {
		ui.PrintSuccess("已取消收藏：%s", h.Alias)
	}
	return nil
}

func (app *App) cmdRecent(args []string) error {
	limit := 10
	if len(args) > 0 {
		parsed, err := strconv.Atoi(args[0])
		if err != nil || parsed < 1 {
			return fmt.Errorf("数量必须是大于 0 的整数")
		}
		limit = parsed
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	rows := make([]indexedHost, 0, len(hf.Hosts))
	for i, host := range hf.Hosts {
		if host.Pinned || host.LastUsedAt != "" {
			rows = append(rows, indexedHost{host: host, index: i})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].host.Pinned != rows[j].host.Pinned {
			return rows[i].host.Pinned
		}
		if rows[i].host.LastUsedAt == rows[j].host.LastUsedAt {
			return rows[i].host.Alias < rows[j].host.Alias
		}
		return rows[i].host.LastUsedAt > rows[j].host.LastUsedAt
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		ui.PrintWarn("暂无最近连接或收藏主机")
		return nil
	}
	hosts, indices := splitIndexedHosts(rows)
	ui.PrintHeader(fmt.Sprintf("收藏与最近连接 · %d 台", len(hosts)))
	ui.RenderHostsTableWithOptions(hosts, ui.HostTableOptions{Indices: indices})
	return nil
}

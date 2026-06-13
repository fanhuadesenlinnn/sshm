package command

import (
	"fmt"
	"sort"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

type indexedHost struct {
	host  config.Host
	index int
}

func (app *App) cmdList(args []string) error {
	group := ""
	sortBy := ""
	options := ui.HostTableOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--compact":
			options.Compact = true
		case "--wide":
			options.Wide = true
		case "--group", "-g":
			if i+1 >= len(args) {
				return fmt.Errorf("选项 %s 缺少分组名", args[i])
			}
			i++
			group = args[i]
		case "--sort":
			if i+1 >= len(args) {
				return fmt.Errorf("选项 --sort 缺少排序字段")
			}
			i++
			sortBy = args[i]
		default:
			return unknownOptionError(args[i])
		}
	}

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	rows := make([]indexedHost, 0, len(hf.Hosts))
	for i, host := range hf.Hosts {
		if group == "" || host.Group == group {
			rows = append(rows, indexedHost{host: host, index: i})
		}
	}
	switch sortBy {
	case "", "id":
	case "alias":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].host.Alias < rows[j].host.Alias })
	case "group":
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].host.Group == rows[j].host.Group {
				return rows[i].host.Alias < rows[j].host.Alias
			}
			return rows[i].host.Group < rows[j].host.Group
		})
	default:
		return fmt.Errorf("不支持的排序字段 %q，可使用 id、alias 或 group", sortBy)
	}

	if len(rows) == 0 {
		ui.PrintWarn("没有匹配的主机，使用 sshm add 添加")
		return nil
	}
	hosts, indices := splitIndexedHosts(rows)
	options.Indices = indices
	ui.PrintHeader("主机列表")
	ui.RenderHostsTableWithOptions(hosts, options)
	return nil
}

func splitIndexedHosts(rows []indexedHost) ([]config.Host, []int) {
	hosts := make([]config.Host, len(rows))
	indices := make([]int, len(rows))
	for i, row := range rows {
		hosts[i] = row.host
		indices[i] = row.index
	}
	return hosts, indices
}

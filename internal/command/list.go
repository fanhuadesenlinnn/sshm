package command

import (
	"fmt"
	"sort"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

type indexedHost struct {
	host  config.Host
	index int
}

func (app *App) cmdList(args []string) error {
	var tagFilters []string
	sortBy := ""
	options := ui.HostTableOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--compact":
			options.Compact = true
		case "--wide":
			options.Wide = true
		case "--tag", "-t":
			if i+1 >= len(args) {
				return fmt.Errorf("选项 %s 缺少标签名", args[i])
			}
			i++
			tagFilters = config.ParseTags(args[i])
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
	if options.Compact && options.Wide {
		return fmt.Errorf("--compact 和 --wide 不能同时使用")
	}

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	rows := make([]indexedHost, 0, len(hf.Hosts))
	for i, host := range hf.Hosts {
		if len(tagFilters) == 0 || host.MatchTags(tagFilters) {
			rows = append(rows, indexedHost{host: host, index: i})
		}
	}
	switch sortBy {
	case "", "id":
	case "alias":
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].host.Alias < rows[j].host.Alias })
	default:
		return fmt.Errorf("不支持的排序字段 %q，可使用 id 或 alias", sortBy)
	}

	if len(rows) == 0 {
		ui.PrintWarn("没有匹配的主机，使用 sshm add 添加")
		return nil
	}
	hosts, indices := splitIndexedHosts(rows)
	options.Indices = indices
	ui.PrintHeader(fmt.Sprintf("主机列表 · %d 台", len(hosts)))
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

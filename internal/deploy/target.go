package deploy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func ResolveTargets(hosts []config.Host, selector TargetSelector) ([]config.Host, error) {
	if selector.Empty() {
		return nil, fmt.Errorf("目标选择器为空")
	}
	if selector.All && (len(selector.Hosts) > 0 || len(selector.Tags) > 0) {
		return nil, fmt.Errorf("all 不能与 hosts 或 tags 混用")
	}
	if selector.All {
		if len(hosts) == 0 {
			return nil, fmt.Errorf("目标选择结果为空")
		}
		return append([]config.Host(nil), hosts...), nil
	}
	hostNames := map[string]bool{}
	for _, raw := range selector.Hosts {
		for _, name := range split(raw) {
			hostNames[name] = true
		}
	}
	tags := []string{}
	for _, raw := range selector.Tags {
		tags = append(tags, split(raw)...)
	}
	tags = unique(tags)
	tagExists := map[string]bool{}
	for _, host := range hosts {
		for _, tag := range host.Tags {
			tagExists[tag] = true
		}
	}
	for _, tag := range tags {
		if !tagExists[tag] {
			return nil, fmt.Errorf("未找到标签或标签没有关联主机: %s", tag)
		}
	}
	var selected []config.Host
	for index, host := range hosts {
		displayID := fmt.Sprintf("%d", index+1)
		matched := hostNames[host.Alias] || hostNames[host.ID] || hostNames[displayID]
		if len(tags) > 0 && host.MatchTags(tags) {
			matched = true
		}
		if matched {
			selected = append(selected, host)
			delete(hostNames, host.Alias)
			delete(hostNames, host.ID)
			delete(hostNames, displayID)
		}
	}
	if len(hostNames) > 0 {
		missing := make([]string, 0, len(hostNames))
		for name := range hostNames {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("未找到主机: %s", strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("目标选择结果为空")
	}
	return selected, nil
}

func split(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
}

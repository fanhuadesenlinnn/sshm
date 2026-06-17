package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func closestString(input string, candidates []string, maxDistance int) (string, bool) {
	best := ""
	bestDistance := maxDistance + 1
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if distance := editDistance(input, candidate); distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best, best != "" && bestDistance <= maxDistance
}

func (app *App) findHost(aliasOrID string) (*config.Host, int, *config.HostsFile, error) {
	h, idx, hf, err := app.Store.FindHost(aliasOrID)
	if err == nil {
		return h, idx, hf, nil
	}
	return nil, -1, nil, app.hostLookupError(aliasOrID, err)
}

func (app *App) hostLookupError(aliasOrID string, cause error) error {
	hf, err := app.Store.Load()
	if err != nil {
		return cause
	}
	candidates := make([]string, 0, len(hf.Hosts)*2)
	for i, host := range hf.Hosts {
		candidates = append(candidates, host.Alias, fmt.Sprintf("%d", i+1))
	}
	if best, ok := closestString(aliasOrID, candidates, 3); ok {
		return fmt.Errorf("未找到主机 %q；你是否想使用 %q？使用 sshm list 查看全部主机: %w", aliasOrID, best, cause)
	}
	return fmt.Errorf("未找到主机 %q；使用 sshm list 查看全部主机: %w", aliasOrID, cause)
}

func missingHostSelectionError(input string, hosts []config.Host) error {
	candidates := make([]string, 0, len(hosts)*2)
	for i, host := range hosts {
		candidates = append(candidates, host.Alias, fmt.Sprintf("%d", i+1))
	}
	if best, ok := closestString(input, candidates, 3); ok {
		return fmt.Errorf("未找到主机 %q；你是否想使用 %q？使用 sshm list 查看全部主机", input, best)
	}
	return fmt.Errorf("未找到主机 %q；使用 sshm list 查看全部主机", input)
}

func missingTagError(name string, tags []config.Tag) error {
	candidates := make([]string, 0, len(tags))
	for _, tag := range tags {
		candidates = append(candidates, tag.Name)
	}
	if best, ok := closestString(name, candidates, 3); ok {
		return fmt.Errorf("未找到标签 %q；你是否想使用 %q？使用 sshm tag list 查看全部标签", name, best)
	}
	return fmt.Errorf("未找到标签 %q；使用 sshm tag list 查看全部标签", name)
}

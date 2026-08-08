package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
)

// Discover returns deploy file paths: explicit --file values, or the default
// deploy.yaml plus deploy.d/*.yaml under SSHMD_HOME.
func Discover(explicit []string) ([]string, error) {
	if len(explicit) > 0 {
		paths := make([]string, 0, len(explicit))
		for _, item := range explicit {
			absolute, err := filepath.Abs(config.ExpandPath(item))
			if err != nil {
				return nil, fmt.Errorf("解析 deploy 文件 %s 失败: %w", item, err)
			}
			paths = append(paths, absolute)
		}
		return unique(paths), nil
	}
	var paths []string
	if exists(config.DeployFilePath()) {
		paths = append(paths, config.DeployFilePath())
	}
	fragments, err := filepath.Glob(filepath.Join(config.DeployDir(), "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(fragments)
	paths = append(paths, fragments...)
	if len(paths) == 0 {
		return nil, fmt.Errorf("未找到 deploy 配置；使用 sshmd deploy init 或通过 --file 指定文件")
	}
	return paths, nil
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

package deployv3

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
)

const factsTTL = time.Hour

// gatherFacts collects minimal host facts, caching them for factsTTL.
func gatherFacts(ctx context.Context, host config.Host, executor ops.Executor, connectTimeout time.Duration, cacheDir string) (Vars, error) {
	cachePath := filepath.Join(cacheDir, safeFactName(host.Alias)+".json")
	if facts, ok := loadFactsCache(cachePath); ok {
		return facts, nil
	}
	result := executor.Exec(ctx, host, ops.ExecOptions{
		Command:        "{ cat /etc/hostname 2>/dev/null || hostname; } | head -1; uname -s; uname -m; grep -E '^(ID|ID_LIKE)=' /etc/os-release 2>/dev/null || true",
		ConnectTimeout: connectTimeout,
	})
	if !result.OK {
		return nil, result.Err
	}
	lines := splitLines(result.Output)
	facts := Vars{}
	if len(lines) > 0 && lines[0] != "" {
		facts["hostname"] = lines[0]
	}
	if len(lines) > 1 {
		facts["system"] = strings.TrimSpace(lines[1])
	}
	if len(lines) > 2 {
		facts["arch"] = strings.TrimSpace(lines[2])
	}
	osID := ""
	for _, line := range lines[3:] {
		if strings.HasPrefix(line, "ID=") {
			osID = strings.Trim(strings.TrimPrefix(line, "ID="), "\"'")
		}
	}
	facts["os_family"] = osFamily(osID)
	facts["os_id"] = osID
	if err := writeFactsCache(cachePath, facts); err != nil {
		return nil, err
	}
	return facts, nil
}

func osFamily(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "":
		return "unknown"
	case "debian", "ubuntu":
		return "debian"
	case "rhel", "centos", "fedora", "rocky", "almalinux", "amzn", "ol":
		return "redhat"
	case "alpine":
		return "alpine"
	case "sles", "opensuse-leap", "opensuse-tumbleweed":
		return "suse"
	default:
		return strings.ToLower(id)
	}
}

func safeFactName(alias string) string {
	var builder strings.Builder
	for _, r := range alias {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

func loadFactsCache(path string) (Vars, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || time.Since(info.ModTime()) > factsTTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var facts Vars
	if err := json.Unmarshal(data, &facts); err != nil {
		return nil, false
	}
	return facts, true
}

func writeFactsCache(path string, facts Vars) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("创建 facts 缓存目录失败: %w", err)
	}
	data, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return err
	}
	if err := safefile.Write(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("写入 facts 缓存失败: %w", err)
	}
	return nil
}

func splitLines(value string) []string {
	return strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
}

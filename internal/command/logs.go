package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdLogs(args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printLogsHelp()
		return nil
	}
	if len(args) > 0 && args[0] == "clean" {
		yes, rest := removeFlag(args[1:], "--yes")
		if len(rest) != 0 {
			return fmt.Errorf("用法: sshmd logs clean [--yes]")
		}
		logsDir, err := safeLogsDirForClean()
		if err != nil {
			return err
		}
		if !yes {
			if !ui.IsTerminal() {
				return fmt.Errorf("清理操作日志需要确认；非交互环境请使用 --yes")
			}
			if !ui.ReadYesNo(fmt.Sprintf("确认清理操作日志目录 %s? [y/N]: ", logsDir)) {
				ui.PrintWarn("已取消")
				return nil
			}
		}
		if err := os.RemoveAll(logsDir); err != nil {
			return err
		}
		ui.PrintSuccess("操作日志已清理")
		return nil
	}
	hostFilter := ""
	actionFilter := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 >= len(args) {
				return fmt.Errorf("--host 缺少别名")
			}
			i++
			hostFilter = args[i]
		case "--action":
			if i+1 >= len(args) {
				return fmt.Errorf("--action 缺少动作")
			}
			i++
			actionFilter = args[i]
		default:
			return fmt.Errorf("未知 logs 命令 %q；使用 sshmd logs help 查看帮助", args[i])
		}
	}
	retention := 30 * 24 * time.Hour
	if doc, err := app.Store.Repository().Load(); err == nil {
		retention = doc.Defaults.Logs.Retention.Duration
	}
	if err := operation.CleanExpired(retention); err != nil {
		return err
	}
	directories := logDirectoriesMatching(actionFilter)
	if len(directories) == 0 {
		ui.PrintWarn("暂无匹配的操作日志")
		return nil
	}
	ui.PrintWarn("操作日志可能包含敏感远程输出，请按本地敏感数据保护")
	for _, dir := range directories {
		if hostFilter != "" {
			for _, match := range hostLogFiles(dir, hostFilter) {
				fmt.Println(match)
			}
			continue
		}
		fmt.Println(dir)
	}
	return nil
}

// logDirectoriesMatching returns run directories whose name matches an action
// filter. Directory names may end in -<action> (like -exec) or contain
// -<action>-<suffix> (like -exec-batch, -push-batch, -deploy-publish).
func logDirectoriesMatching(action string) []string {
	entries, err := os.ReadDir(config.LogsDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if action != "" && !logNameMatchesAction(name, action) {
			continue
		}
		out = append(out, filepath.Join(config.LogsDir(), name))
	}
	return out
}

func logNameMatchesAction(name, action string) bool {
	return strings.HasSuffix(name, "-"+action) || strings.Contains(name, "-"+action+"-")
}

// hostLogFiles returns the per-host log files of a run directory matching the
// alias. Host aliases are restricted to [A-Za-z0-9._-], so the alias itself
// is a safe filename prefix.
func hostLogFiles(dir, alias string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := alias + "-"
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			matches = append(matches, filepath.Join(dir, entry.Name()))
		}
	}
	return matches
}

func safeLogsDirForClean() (string, error) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return "", err
	}
	home := filepath.Clean(paths.Home)
	logsDir := filepath.Clean(paths.Logs)
	if home == "" || logsDir == "" || filepath.Dir(home) == home {
		return "", fmt.Errorf("拒绝清理日志：SSHMD_HOME 不能是文件系统根目录")
	}
	if filepath.Base(logsDir) != "logs" {
		return "", fmt.Errorf("拒绝清理日志：日志目录必须以 logs 结尾: %s", logsDir)
	}
	rel, err := filepath.Rel(home, logsDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("拒绝清理日志：日志目录不在 SSHMD_HOME 内: %s", logsDir)
	}
	return logsDir, nil
}

func printLogsHelp() {
	ui.PrintHeader("操作日志")
	fmt.Println()
	fmt.Println("  logs                  列出日志目录")
	fmt.Println("  logs --host <别名>    只看某台主机的日志")
	fmt.Println("  logs --action <动作>  只看某个动作（exec/exec-batch/deploy/key-*）")
	fmt.Println("  logs clean [--yes]    清理本地操作日志")
	fmt.Println()
	fmt.Println("  日志可能包含敏感远程输出，默认保存在 ~/.sshmd/logs。")
	fmt.Println()
}

package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdLogs(args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printLogsHelp()
		return nil
	}
	if len(args) > 0 && args[0] == "clean" {
		yes, rest := removeFlag(args[1:], "--yes")
		if len(rest) != 0 {
			return fmt.Errorf("用法: sshm logs clean [--yes]")
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
	if len(args) > 0 {
		return fmt.Errorf("未知 logs 命令 %q；使用 sshm logs help 查看帮助", args[0])
	}
	retention := 30 * 24 * time.Hour
	if doc, err := app.Store.Repository().Load(); err == nil {
		retention = doc.Defaults.Logs.Retention.Duration
	}
	if err := operation.CleanExpired(retention); err != nil {
		return err
	}
	entries, err := os.ReadDir(config.LogsDir())
	if os.IsNotExist(err) {
		ui.PrintWarn("暂无操作日志")
		return nil
	}
	if err != nil {
		return err
	}
	ui.PrintWarn("操作日志可能包含敏感远程输出，请按本地敏感数据保护")
	for _, entry := range entries {
		if entry.IsDir() {
			fmt.Println(filepath.Join(config.LogsDir(), entry.Name()))
		}
	}
	return nil
}

func safeLogsDirForClean() (string, error) {
	paths, err := config.ResolvePaths()
	if err != nil {
		return "", err
	}
	home := filepath.Clean(paths.Home)
	logsDir := filepath.Clean(paths.Logs)
	if home == "" || logsDir == "" || filepath.Dir(home) == home {
		return "", fmt.Errorf("拒绝清理日志：SSHM_HOME 不能是文件系统根目录")
	}
	if filepath.Base(logsDir) != "logs" {
		return "", fmt.Errorf("拒绝清理日志：日志目录必须以 logs 结尾: %s", logsDir)
	}
	rel, err := filepath.Rel(home, logsDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("拒绝清理日志：日志目录不在 SSHM_HOME 内: %s", logsDir)
	}
	return logsDir, nil
}

func printLogsHelp() {
	ui.PrintHeader("操作日志")
	fmt.Println()
	fmt.Println("  logs                  列出日志目录")
	fmt.Println("  logs clean [--yes]    清理本地操作日志")
	fmt.Println()
	fmt.Println("  日志可能包含敏感远程输出，默认保存在 ~/.sshm/logs。")
	fmt.Println()
}

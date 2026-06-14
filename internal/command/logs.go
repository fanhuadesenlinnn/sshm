package command

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

func (app *App) cmdLogs(args []string) error {
	if len(args) > 0 && args[0] == "clean" {
		if err := os.RemoveAll(config.LogsDir()); err != nil {
			return err
		}
		ui.PrintSuccess("操作日志已清理")
		return nil
	}
	if err := operation.CleanExpired(30 * 24 * time.Hour); err != nil {
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

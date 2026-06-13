package command

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdDoctor(_ []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return fmt.Errorf("读取主机配置失败: %w", err)
	}

	ui.PrintHeader("sshm 环境检查")
	fmt.Println()
	fmt.Printf("  %-14s %s\n", "版本", Version)
	fmt.Printf("  %-14s %s\n", "配置文件", app.Store.Path())
	fmt.Printf("  %-14s %s\n", "密码文件", app.SecretPath)
	fmt.Printf("  %-14s %s\n", "密钥目录", config.KeysDir())
	fmt.Printf("  %-14s %d\n", "主机数量", len(hf.Hosts))

	if sshPath, err := exec.LookPath("ssh"); err == nil {
		fmt.Printf("  %-14s %s\n", "系统 SSH", sshPath)
	} else {
		ui.PrintWarn("未找到系统 ssh 命令，system/auto 连接可能不可用")
	}

	missingKeys := 0
	for _, host := range hf.Hosts {
		if host.Identity == "" {
			continue
		}
		if _, err := os.Stat(config.ExpandPath(host.Identity)); err != nil {
			missingKeys++
			ui.PrintWarn("%s 的密钥不存在: %s", host.Alias, config.ExpandPath(host.Identity))
		}
	}
	if missingKeys == 0 {
		ui.PrintSuccess("环境检查完成，未发现明显问题")
	} else {
		ui.PrintWarn("环境检查完成：%d 个密钥路径需要处理", missingKeys)
	}
	return nil
}

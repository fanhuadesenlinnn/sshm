package command

import (
	"fmt"

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
	fmt.Printf("  %-14s %s\n", "托管密钥", app.keyStore().Path())
	fmt.Printf("  %-14s %s\n", "密钥目录", config.KeysDir())
	fmt.Printf("  %-14s %d\n", "主机数量", len(hf.Hosts))

	missingKeys := 0
	managedKeys := 0
	for _, host := range hf.Hosts {
		if host.Identity == "" {
			continue
		}
		if name, managed := config.ManagedKeyName(host.Identity); managed {
			if _, err := app.keyStore().Find(name); err != nil {
				missingKeys++
				ui.PrintWarn("%s 的托管密钥不可用: %v", host.Alias, err)
			} else {
				managedKeys++
			}
		}
	}
	if missingKeys == 0 {
		ui.PrintSuccess("环境检查完成，未发现明显问题（%d 台主机使用托管密钥）", managedKeys)
	} else {
		ui.PrintWarn("环境检查完成：%d 个密钥路径需要处理", missingKeys)
	}
	return nil
}

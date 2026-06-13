package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
)

func (app *App) cmdDoctor(_ []string) error {
	hf, err := app.Store.Load()
	if err != nil {
		return fmt.Errorf("读取主机配置失败: %w", err)
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return fmt.Errorf("读取完整配置失败: %w", err)
	}

	ui.PrintHeader("sshm 环境检查")
	fmt.Println()
	fmt.Printf("  %-14s %s\n", "版本", CurrentVersion())
	fmt.Printf("  %-14s %s\n", "配置文件", app.Store.Path())
	fmt.Printf("  %-14s %d\n", "主机数量", len(hf.Hosts))

	missingKeys := 0
	managedKeys := 0
	insecureHosts := 0
	passwordRefs := 0
	for _, host := range hf.Hosts {
		if host.ResolvedHostKeyPolicy == config.HostKeyPolicyInsecure {
			insecureHosts++
			ui.PrintWarn("%s 跳过主机身份验证", host.Alias)
		}
		if host.Identity == "" {
			if host.PasswordRef != "" {
				passwordRefs++
			}
			continue
		}
		if host.PasswordRef != "" {
			passwordRefs++
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
	credentialIssues := 0
	if doc.Vault == nil && (passwordRefs > 0 || len(doc.ManagedKeys.Keys) > 0) {
		credentialIssues++
		ui.PrintWarn("配置引用了凭据，但 vault 尚不存在")
	} else if doc.Vault != nil {
		if fs := app.tryGetSecretStore(); fs != nil {
			for _, host := range hf.Hosts {
				if host.PasswordRef != "" {
					if _, err := fs.GetPassword(host.PasswordRef); err != nil {
						credentialIssues++
						ui.PrintWarn("%s 的保存密码不可用: %v", host.Alias, err)
					}
				}
			}
			for _, key := range doc.ManagedKeys.Keys {
				if _, err := fs.GetManagedKey(key.Name); err != nil {
					credentialIssues++
					ui.PrintWarn("托管密钥 %s 的私钥不可用: %v", key.Name, err)
				}
			}
		}
	}
	if missingKeys == 0 && credentialIssues == 0 {
		ui.PrintSuccess("环境检查完成（%d 台使用托管密钥，%d 台跳过身份验证）", managedKeys, insecureHosts)
	} else {
		ui.PrintWarn("环境检查完成：%d 个密钥引用、%d 个凭据问题需要处理", missingKeys, credentialIssues)
	}
	return nil
}

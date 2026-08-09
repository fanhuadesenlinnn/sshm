package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdDoctor(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("用法: sshmd doctor")
	}
	hf, err := app.Store.Load()
	if errors.Is(err, config.ErrNotInitialized) {
		ui.PrintHeader("sshmd 环境检查")
		fmt.Println()
		fmt.Printf("  %-14s %s\n", "版本", CurrentVersion())
		fmt.Printf("  %-14s %s\n", "配置文件", app.Store.Path())
		ui.PrintWarn("sshmd 尚未初始化；请运行 sshmd init")
		if legacy, exists := config.LegacyConfigExists(); exists {
			ui.PrintWarn("发现旧配置 %s；v6 不会读取、迁移或删除该文件", legacy)
		}
		return &ExitError{Code: 3, Err: fmt.Errorf("环境检查未通过：sshmd 尚未初始化")}
	}
	if err != nil {
		return fmt.Errorf("读取主机配置失败: %w", err)
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return fmt.Errorf("读取完整配置失败: %w", err)
	}
	ui.PrintHeader("sshmd 环境检查")
	fmt.Println()
	fmt.Printf("  %-14s %s\n", "版本", CurrentVersion())
	fmt.Printf("  %-14s %s\n", "配置文件", app.Store.Path())
	fmt.Printf("  %-14s %d\n", "主机数量", len(hf.Hosts))

	configIssues := app.checkDeployConfig()
	vaultIssues := 0
	warnings := 0
	managedKeys := 0
	insecureHosts := 0
	passwordRefs := 0
	for _, host := range hf.Hosts {
		if host.ResolvedHostKeyPolicy == config.HostKeyPolicyInsecure {
			insecureHosts++
			warnings++
			ui.PrintWarn("%s 跳过主机身份验证", host.Alias)
		}
		if host.PasswordRef != "" {
			passwordRefs++
		}
		if _, managed := config.ManagedKeyName(host.Identity); managed {
			managedKeys++
		}
		hasKey := host.Auth != "password" && host.Identity != ""
		hasPassword := host.Auth != "key" && (host.Password != "" || host.PasswordRef != "")
		if !hasKey && !hasPassword {
			configIssues++
			ui.PrintWarn("%s 未配置符合 auth=%s 的认证凭据；请运行 sshmd passwd 或 sshmd key setup", host.Alias, host.Auth)
		}
	}
	if doc.Vault == nil && (passwordRefs > 0 || len(doc.ManagedKeys.Keys) > 0) {
		vaultIssues++
		ui.PrintWarn("配置引用了凭据，但 vault 尚不存在")
	} else if doc.Vault != nil {
		fs, unlockErr := app.doctorSecretStore()
		if unlockErr != nil {
			vaultIssues++
			ui.PrintWarn("vault 无法验证: %v", unlockErr)
		} else {
			for _, host := range hf.Hosts {
				if host.PasswordRef != "" {
					if _, err := fs.GetPassword(host.PasswordRef); err != nil {
						vaultIssues++
						ui.PrintWarn("%s 的保存密码不可用: %v", host.Alias, err)
					}
				}
			}
			for _, key := range doc.ManagedKeys.Keys {
				if _, err := fs.GetManagedKey(key.Name); err != nil {
					vaultIssues++
					ui.PrintWarn("托管密钥 %s 的私钥不可用: %v", key.Name, err)
				}
			}
		}
	}
	plaintext := 0
	for _, host := range hf.Hosts {
		if host.Password != "" {
			plaintext++
		}
	}
	if plaintext > 0 {
		warnings++
		ui.PrintWarn("%d 台主机使用主配置明文密码；建议用 sshmd passwd 加密到 vault", plaintext)
	}
	if configIssues == 0 && vaultIssues == 0 {
		ui.PrintSuccess("环境检查通过（%d 台使用托管密钥，%d 项安全提醒）", managedKeys, warnings)
		return nil
	}
	ui.PrintWarn("环境检查未通过：%d 个配置问题、%d 个 vault/凭据问题", configIssues, vaultIssues)
	code := 3
	if configIssues == 0 {
		code = 4
	}
	return &ExitError{Code: code, Err: fmt.Errorf("环境检查发现需要处理的问题")}
}

func (app *App) doctorSecretStore() (*secret.FileStore, error) {
	if app.secretStore != nil {
		return app.secretStore, nil
	}
	if fs, ok, err := app.secretStoreFromEnv(); ok || err != nil {
		return fs, err
	}
	if !ui.IsTerminal() {
		return nil, fmt.Errorf("非交互环境未设置 %s，无法验证 vault", masterPasswordEnv)
	}
	return app.requireSecretStore()
}

// checkDeployConfig returns one issue for each unusable deploy catalog;
// missing deploy files are normal on fresh installs.
func (app *App) checkDeployConfig() int {
	paths, err := deploy.Discover(nil)
	if err != nil {
		if strings.Contains(err.Error(), "未找到 deploy 配置") {
			return 0
		}
		ui.PrintWarn("Deploy 配置发现失败: %v", err)
		return 1
	}
	catalog, err := deploy.Load(paths)
	if err != nil {
		ui.PrintWarn("Deploy 配置无法加载: %v", err)
		return 1
	}
	if err := deploy.ValidateCatalog(catalog); err != nil {
		ui.PrintWarn("Deploy 配置校验失败: %v", err)
		return 1
	}
	return 0
}

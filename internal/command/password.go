package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdPasswd(args []string) error {
	yes, args := removeFlag(args, "--yes")
	hosts, err := app.resolveCredentialTargets(args)
	if err != nil {
		return err
	}
	if len(hosts) > 1 && !yes {
		printCredentialTargets("设置同一个 SSH 密码", hosts)
		if !ui.IsTerminal() {
			return fmt.Errorf("批量设置密码需要确认；非交互环境请使用 --yes")
		}
		if !ui.ReadYesNo("确认设置? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	fs, fsErr := app.requireSecretStore()
	if fsErr != nil {
		return fmt.Errorf("无法访问密码存储: %w", fsErr)
	}
	if fs == nil {
		return fmt.Errorf("无法访问密码存储")
	}
	password, err := readConfirmedSSHPassword()
	if err != nil {
		return err
	}

	if err := savePasswordsForHosts(fs, hosts, password); err != nil {
		return fmt.Errorf("保存密码失败: %w", err)
	}
	if len(hosts) == 1 {
		ui.PrintSuccess("密码已加密保存：%s", hosts[0].Alias)
	} else {
		ui.PrintSuccess("密码已加密保存：%d 台主机", len(hosts))
	}
	return nil
}

func savePasswordsForHosts(fs *secret.FileStore, hosts []config.Host, password string) error {
	selected := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		selected[host.ID] = true
	}
	updated := 0
	return fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		for i := range doc.Hosts {
			host := &doc.Hosts[i]
			if !selected[host.ID] {
				continue
			}
			delete(entries, "password:"+host.Alias)
			if host.PasswordRef != "" && host.PasswordRef != host.ID {
				delete(entries, "password:"+host.PasswordRef)
			}
			entries["password:"+host.ID] = password
			host.PasswordRef = host.ID
			if host.Auth == "" {
				host.Auth = "auto"
			}
			updated++
		}
		if updated != len(selected) {
			return fmt.Errorf("目标主机在更新期间发生变化，期望 %d 台，找到 %d 台", len(selected), updated)
		}
		return nil
	})
}

func (app *App) cmdForgetPass(args []string) error {
	yes, args := removeFlag(args, "--yes")
	hosts, err := app.resolveCredentialTargets(args)
	if err != nil {
		return err
	}
	withPassword := make([]config.Host, 0, len(hosts))
	for _, host := range hosts {
		if host.PasswordRef != "" {
			withPassword = append(withPassword, host)
		}
	}
	if len(withPassword) == 0 {
		ui.PrintWarn("所选主机都没有保存的密码")
		return nil
	}

	if !yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("删除保存密码需要确认；非交互环境请使用 --yes")
		}
		if len(withPassword) > 1 {
			printCredentialTargets("删除已保存的 SSH 密码", withPassword)
		}
		if !ui.ReadYesNo(fmt.Sprintf("确认删除 %d 台主机的密码? [y/N]: ", len(withPassword))) {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	fs, fsErr := app.requireSecretStore()
	if fsErr != nil {
		return fmt.Errorf("无法访问密码存储: %w", fsErr)
	}
	if fs == nil {
		return fmt.Errorf("无法访问密码存储")
	}
	if err := removePasswordsForHosts(fs, withPassword); err != nil {
		return fmt.Errorf("删除密码失败: %w", err)
	}

	if len(withPassword) == 1 {
		ui.PrintSuccess("密码已删除：%s", withPassword[0].Alias)
	} else {
		ui.PrintSuccess("密码已删除：%d 台主机", len(withPassword))
	}
	if skipped := len(hosts) - len(withPassword); skipped > 0 {
		ui.PrintWarn("另有 %d 台主机未保存密码，已跳过", skipped)
	}
	return nil
}

func removePasswordsForHosts(fs *secret.FileStore, hosts []config.Host) error {
	selected := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		selected[host.ID] = true
	}
	updated := 0
	return fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		for i := range doc.Hosts {
			host := &doc.Hosts[i]
			if !selected[host.ID] {
				continue
			}
			delete(entries, "password:"+host.Alias)
			delete(entries, "password:"+host.ID)
			delete(entries, "password:"+host.PasswordRef)
			host.PasswordRef = ""
			if host.Auth == "password" {
				host.Auth = "auto"
			}
			updated++
		}
		if updated != len(selected) {
			return fmt.Errorf("目标主机在更新期间发生变化，期望 %d 台，找到 %d 台", len(selected), updated)
		}
		return nil
	})
}

func (app *App) resolveCredentialTargets(args []string) ([]config.Host, error) {
	if len(args) == 0 {
		input := ui.ReadLine("请输入主机别名或ID（多个目标用空格分隔，也支持 --tag/--all）: ")
		args = parseArgs(input)
	}
	return app.selectHosts(args)
}

func printCredentialTargets(action string, hosts []config.Host) {
	fmt.Printf("\n即将%s到 %d 台主机:\n", action, len(hosts))
	for _, host := range hosts {
		fmt.Printf("  - %s (%s@%s:%d)\n", host.Alias, host.User, host.Host, host.Port)
	}
	fmt.Println()
}

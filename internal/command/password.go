package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
)

func (app *App) cmdPasswd(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}

	fs, fsErr := app.requireSecretStore()
	if fsErr != nil {
		return fmt.Errorf("无法访问密码存储: %w", fsErr)
	}
	if fs == nil {
		return fmt.Errorf("无法访问密码存储")
	}
	pass1, err := ui.ReadPassword("请输入 SSH 密码: ")
	if err != nil {
		return fmt.Errorf("读取密码失败: %w", err)
	}
	pass2, err := ui.ReadPassword("再次输入 SSH 密码: ")
	if err != nil {
		return fmt.Errorf("读取密码失败: %w", err)
	}

	if pass1 != pass2 {
		return fmt.Errorf("两次密码不一致")
	}

	if err := fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID != h.ID {
				continue
			}
			delete(entries, "password:"+h.Alias)
			entries["password:"+h.ID] = pass1
			doc.Hosts[i].PasswordRef = h.ID
			if doc.Hosts[i].Auth == "" {
				doc.Hosts[i].Auth = "auto"
			}
			return nil
		}
		return fmt.Errorf("未找到主机: %s", h.Alias)
	}); err != nil {
		return fmt.Errorf("保存密码失败: %w", err)
	}

	ui.PrintSuccess("密码已加密保存：%s", h.Alias)
	return nil
}

func (app *App) cmdForgetPass(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}
	if h.PasswordRef == "" {
		ui.PrintWarn("主机 %s 没有保存的密码", h.Alias)
		return nil
	}

	confirm := ui.ReadLine(fmt.Sprintf("确认删除 %s 的密码? [y/N]: ", h.Alias))
	if confirm != "y" && confirm != "Y" && confirm != "yes" {
		ui.PrintWarn("已取消")
		return nil
	}

	fs, fsErr := app.requireSecretStore()
	if fsErr != nil {
		return fmt.Errorf("无法访问密码存储: %w", fsErr)
	}
	if fs == nil {
		return fmt.Errorf("无法访问密码存储")
	}
	if err := fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID != h.ID {
				continue
			}
			delete(entries, "password:"+h.Alias)
			delete(entries, "password:"+h.ID)
			delete(entries, "password:"+h.PasswordRef)
			doc.Hosts[i].PasswordRef = ""
			if doc.Hosts[i].Auth == "password" {
				doc.Hosts[i].Auth = "auto"
			}
			return nil
		}
		return fmt.Errorf("未找到主机: %s", h.Alias)
	}); err != nil {
		return fmt.Errorf("删除密码失败: %w", err)
	}

	ui.PrintSuccess("密码已删除：%s", h.Alias)
	return nil
}

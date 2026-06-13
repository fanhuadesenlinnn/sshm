package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdPasswd(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入主机别名或ID: ")
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
	h, idx, hf, err = app.Store.FindHost(h.Alias)
	if err != nil {
		return fmt.Errorf("重新加载迁移后的主机配置失败: %w", err)
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

	// Save by stable ID if available, else by alias
	if h.ID != "" {
		if err := fs.SetPasswordByID(h.ID, h.Alias, pass1); err != nil {
			return fmt.Errorf("保存密码失败: %w", err)
		}
		hf.Hosts[idx].PasswordRef = h.ID
	} else {
		if err := fs.SetPassword(h.Alias, pass1); err != nil {
			return fmt.Errorf("保存密码失败: %w", err)
		}
		hf.Hosts[idx].PasswordRef = h.Alias
	}

	if hf.Hosts[idx].Auth == "" {
		hf.Hosts[idx].Auth = "auto"
	}

	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("更新主机配置失败: %w", err)
	}

	ui.PrintSuccess("密码已加密保存：%s", h.Alias)
	return nil
}

func (app *App) cmdForgetPass(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入主机别名或ID: ")
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
	h, idx, hf, err = app.Store.FindHost(h.Alias)
	if err != nil {
		return fmt.Errorf("重新加载迁移后的主机配置失败: %w", err)
	}

	hf.Hosts[idx].PasswordRef = ""
	if hf.Hosts[idx].Auth == "password" {
		hf.Hosts[idx].Auth = "auto"
	}
	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("更新主机配置失败: %w", err)
	}
	if err := fs.RemovePasswords(h.Alias, h.ID, h.PasswordRef); err != nil {
		return fmt.Errorf("主机配置已更新，但清理保存密码失败: %w", err)
	}

	ui.PrintSuccess("密码已删除：%s", h.Alias)
	return nil
}

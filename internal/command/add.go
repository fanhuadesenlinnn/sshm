package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/keymgr"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdAdd(args []string) error {
	fmt.Println()
	ui.PrintHeader("添加新的 SSH 主机")
	fmt.Println()

	h := config.DefaultHost()

	h.Alias = ui.ReadLine("别名: ")
	if h.Alias == "" {
		return fmt.Errorf("别名不能为空")
	}

	h.User = ui.ReadLineDefault("用户 [root]: ", "root")
	h.Host = ui.ReadLine("主机/IP: ")
	if h.Host == "" {
		return fmt.Errorf("主机/IP 不能为空")
	}

	portStr := ui.ReadLineDefault("端口 [22]: ", "22")
	fmt.Sscanf(portStr, "%d", &h.Port)
	if h.Port < 1 || h.Port > 65535 {
		h.Port = 22
	}

	identityInput := ui.ReadLine("密钥路径，可留空: ")
	h.Identity = identityInput

	// If identity path provided, offer to import
	if identityInput != "" {
		if ui.ReadYesNo("是否导入密钥到 sshm keys 目录？[y/N]: ") {
			relPath, err := keymgr.ImportKey(h.Alias, identityInput)
			if err != nil {
				ui.PrintWarn("导入密钥失败: %v", err)
			} else {
				h.Identity = relPath
			}
		}
	}

	// Prompt for SSH password first, then ask if save
	var sshPassword string
	passInput, err := ui.ReadPassword("请输入 SSH 密码 (留空跳过): ")
	if err != nil {
		ui.PrintWarn("读取密码失败，将跳过密码设置")
	} else if passInput != "" {
		pass2, err := ui.ReadPassword("再次输入 SSH 密码: ")
		if err != nil {
			ui.PrintWarn("读取密码失败，将跳过密码设置")
		} else if passInput != pass2 {
			ui.PrintWarn("两次密码不一致，将跳过密码设置")
		} else {
			sshPassword = passInput
		}
	}

	savePass := false
	if sshPassword != "" {
		savePass = ui.ReadYesNo("是否保存此密码？[y/N]: ")
	}

	// Auth strategy selection
	fmt.Println()
	fmt.Println("  认证策略:")
	fmt.Println("    auto     - 自动选择（密钥优先，密码备用）")
	fmt.Println("    key      - 仅密钥认证")
	fmt.Println("    password - 仅密码认证")
	fmt.Println("    ask      - 每次询问密码")
	fmt.Println("    system   - 使用系统 SSH 默认行为")
	fmt.Println()
	newAuth := ui.ReadLineDefault("认证策略 [auto]: ", "auto")
	validAuth := map[string]bool{"auto": true, "key": true, "password": true, "ask": true, "system": true}
	if !validAuth[newAuth] {
		ui.PrintWarn("无效的认证策略 '%s'，使用默认值 'auto'", newAuth)
		newAuth = "auto"
	}
	h.Auth = newAuth

	// If user picked password auth but didn't save password, warn
	if newAuth == "password" && !savePass {
		ui.PrintWarn("选择了密码认证但未保存密码，将回退到系统 SSH")
	}

	h.Note = ui.ReadLine("备注，可留空: ")
	h.Group = ui.ReadLine("分组，可留空: ")

	tagsInput := ui.ReadLine("标签（多个用空格或英文逗号分隔），可留空: ")
	h.Tags = config.ParseTags(tagsInput)

	// Validate
	if errs := h.Validate(); len(errs) > 0 {
		for _, e := range errs {
			ui.PrintError("%s", e)
		}
		return fmt.Errorf("输入校验失败")
	}

	// Save host first
	if err := app.Store.Add(h); err != nil {
		return fmt.Errorf("添加主机失败: %w", err)
	}

	// Save password if requested
	if savePass && sshPassword != "" {
		fs := app.tryGetSecretStore()
		if fs != nil {
			if err := fs.SetPasswordByID(h.ID, h.Alias, sshPassword); err != nil {
				ui.PrintWarn("保存密码失败: %v", err)
			} else {
				// Update host with password_ref using stable ID
				hf, _ := app.Store.Load()
				for i := range hf.Hosts {
					if hf.Hosts[i].Alias == h.Alias {
						hf.Hosts[i].PasswordRef = h.ID
						_ = app.Store.Save(hf)
						break
					}
				}
			}
		}
	}

	// Print summary
	fmt.Println()
	ui.PrintSuccess("已添加主机：%s", h.Alias)
	fmt.Printf("  %s\n", ui.Info("认证策略", h.Auth))
	if h.Identity != "" {
		fmt.Printf("  %s\n", ui.Info("密钥", h.Identity))
	} else {
		fmt.Printf("  %s\n", ui.Info("密钥", "无"))
	}
	if savePass {
		fmt.Printf("  %s\n", ui.Info("密码", "已加密保存"))
	} else {
		fmt.Printf("  %s\n", ui.Info("密码", "未保存"))
	}
	fmt.Println()

	return nil
}

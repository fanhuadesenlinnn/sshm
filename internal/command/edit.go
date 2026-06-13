package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdEdit(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入要编辑的主机别名或ID: ")
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintHeader(fmt.Sprintf("编辑主机: %s (留空保留原值，输入 - 清空可选字段)", h.Alias))
	fmt.Println()

	newAlias := ui.ReadLineDefault(fmt.Sprintf("别名 [%s]: ", h.Alias), h.Alias)
	newUser := ui.ReadLineDefault(fmt.Sprintf("用户 [%s]: ", h.User), h.User)
	newHost := ui.ReadLineDefault(fmt.Sprintf("主机/IP [%s]: ", h.Host), h.Host)

	portStr := ui.ReadLineDefault(fmt.Sprintf("端口 [%d]: ", h.Port), fmt.Sprintf("%d", h.Port))
	newPort := h.Port
	fmt.Sscanf(portStr, "%d", &newPort)

	newIdentity := readEditableValue("密钥路径", h.Identity)
	newNote := readEditableValue("备注", h.Note)
	newGroup := readEditableValue("分组", h.Group)

	newAuth := ui.ReadLineDefault(fmt.Sprintf("认证策略 (auto/key/password/ask/system) [%s]: ", h.Auth), h.Auth)

	tagsInput := readEditableValue("标签", config.TagsToString(h.Tags))
	newTags := config.ParseTags(tagsInput)

	aliasChanged := newAlias != h.Alias

	updated := config.Host{
		ID:          h.ID,
		Alias:       newAlias,
		User:        newUser,
		Host:        newHost,
		Port:        newPort,
		Identity:    newIdentity,
		Note:        newNote,
		Group:       newGroup,
		Tags:        newTags,
		Auth:        newAuth,
		PasswordRef: h.PasswordRef,
		Pinned:      h.Pinned,
		LastUsedAt:  h.LastUsedAt,
	}
	if errs := updated.Validate(); len(errs) > 0 {
		for _, e := range errs {
			ui.PrintError("%s", e)
		}
		return fmt.Errorf("输入校验失败")
	}
	for i, existing := range hf.Hosts {
		if i != idx && existing.Alias == newAlias {
			return fmt.Errorf("别名 '%s' 已存在", newAlias)
		}
	}

	if h.PasswordRef != "" {
		fmt.Printf("\n当前主机已保存密码。\n")
		changePass := ui.ReadYesNo("是否修改密码？[y/N]: ")
		if changePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				return fmt.Errorf("无法访问密码存储: %w", fsErr)
			} else if fs != nil {
				if err := app.changeHostPasswordWithStore(fs, h.ID, h.Alias); err != nil {
					return fmt.Errorf("修改密码失败: %w", err)
				} else {
					updated.PasswordRef = h.ID
					ui.PrintSuccess("密码已更新")
				}
			}
		} else if aliasChanged && h.PasswordRef != h.ID {
			fs := app.tryGetSecretStore()
			if fs != nil {
				if _, err := fs.GetPassword(h.ID); err == nil {
					updated.PasswordRef = h.ID
				}
			}
		}
	} else {
		savePass := ui.ReadYesNo("\n是否保存 SSH 密码？[y/N]: ")
		if savePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				return fmt.Errorf("无法访问密码存储: %w", fsErr)
			} else if fs != nil {
				if err := app.changeHostPasswordWithStore(fs, h.ID, h.Alias); err != nil {
					return fmt.Errorf("保存密码失败: %w", err)
				} else {
					updated.PasswordRef = h.ID
					ui.PrintSuccess("密码已加密保存")
				}
			}
		}
	}

	if err := app.Store.Update(idx, updated); err != nil {
		return err
	}

	ui.PrintSuccess("已更新主机：%s", newAlias)
	return nil
}

func readEditableValue(label, current string) string {
	display := current
	if display == "" {
		display = "空"
	}
	value := ui.ReadLine(fmt.Sprintf("%s [%s]（留空保留，输入 - 清空）: ", label, display))
	if value == "" {
		return current
	}
	if value == "-" {
		return ""
	}
	return value
}

func (app *App) changeHostPasswordWithStore(fs *secret.FileStore, id, alias string) error {
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

	if err := fs.SetPasswordByID(id, alias, pass1); err != nil {
		return fmt.Errorf("保存密码失败: %w", err)
	}

	return nil
}

package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/secret"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdEdit(args []string) error {
	h, idx, _, err := app.resolveHost(args, "请输入要编辑的主机别名或ID: ")
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintHeader(fmt.Sprintf("编辑主机: %s (留空保留原值)", h.Alias))
	fmt.Println()

	newAlias := ui.ReadLineDefault(fmt.Sprintf("别名 [%s]: ", h.Alias), h.Alias)
	newUser := ui.ReadLineDefault(fmt.Sprintf("用户 [%s]: ", h.User), h.User)
	newHost := ui.ReadLineDefault(fmt.Sprintf("主机/IP [%s]: ", h.Host), h.Host)

	portStr := ui.ReadLineDefault(fmt.Sprintf("端口 [%d]: ", h.Port), fmt.Sprintf("%d", h.Port))
	newPort := h.Port
	fmt.Sscanf(portStr, "%d", &newPort)

	newIdentity := ui.ReadLineDefault(fmt.Sprintf("密钥路径 [%s]: ", h.Identity), h.Identity)
	newNote := ui.ReadLineDefault(fmt.Sprintf("备注 [%s]: ", h.Note), h.Note)
	newGroup := ui.ReadLineDefault(fmt.Sprintf("分组 [%s]: ", h.Group), h.Group)

	newAuth := ui.ReadLineDefault(fmt.Sprintf("认证策略 (auto/key/password/ask/system) [%s]: ", h.Auth), h.Auth)

	tagsInput := ui.ReadLineDefault(fmt.Sprintf("标签 [%s]: ", config.TagsToString(h.Tags)), config.TagsToString(h.Tags))
	newTags := config.ParseTags(tagsInput)

	passwordRef := h.PasswordRef
	oldAlias := h.Alias
	aliasChanged := newAlias != oldAlias

	if h.PasswordRef != "" {
		fmt.Printf("\n当前主机已保存密码。\n")
		changePass := ui.ReadYesNo("是否修改密码？[y/N]: ")
		if changePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				ui.PrintWarn("无法访问密码存储: %v", fsErr)
			} else if fs != nil {
				if err := app.changeHostPasswordWithStore(fs, h.Alias); err != nil {
					ui.PrintWarn("修改密码失败: %v", err)
				} else {
					passwordRef = h.ID
					ui.PrintSuccess("密码已更新")
				}
			}
		} else if aliasChanged {
			fs := app.tryGetSecretStore()
			if fs != nil {
				if migrated, _ := fs.MigrateAliasToID(oldAlias, h.ID); migrated {
					passwordRef = h.ID
					ui.PrintSuccess("密码引用已迁移到新别名")
				}
			}
		}
	} else {
		savePass := ui.ReadYesNo("\n是否保存 SSH 密码？[y/N]: ")
		if savePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				ui.PrintWarn("无法访问密码存储: %v", fsErr)
			} else if fs != nil {
				if err := app.changeHostPasswordWithStore(fs, h.Alias); err != nil {
					ui.PrintWarn("保存密码失败: %v", err)
				} else {
					passwordRef = h.ID
					ui.PrintSuccess("密码已加密保存")
				}
			}
		}
	}

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
		PasswordRef: passwordRef,
	}

	if errs := updated.Validate(); len(errs) > 0 {
		for _, e := range errs {
			ui.PrintError("%s", e)
		}
		return fmt.Errorf("输入校验失败")
	}

	if err := app.Store.Update(idx, updated); err != nil {
		return err
	}

	ui.PrintSuccess("已更新主机：%s", newAlias)
	return nil
}

func (app *App) changeHostPasswordWithStore(fs *secret.FileStore, alias string) error {
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

	if err := fs.SetPassword(alias, pass1); err != nil {
		return fmt.Errorf("保存密码失败: %w", err)
	}

	return nil
}

package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
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

	newIdentity := normalizeManagedIdentity(readEditableValue("托管密钥", managedIdentityDisplay(h.Identity)))
	newNote := readEditableValue("备注", h.Note)

	newAuth := ui.ReadLineDefault(fmt.Sprintf("认证策略 (auto/key/password) [%s]: ", h.Auth), h.Auth)
	newHostKeyPolicy := readEditableValue("主机信任策略 (strict/accept-new/insecure，空为继承)", h.HostKeyPolicy)
	newJumpHost := readEditableValue("跳板机别名", h.JumpHost)

	tagsInput := readEditableValue("标签", config.TagsToString(h.Tags))
	newTags := config.ParseTags(tagsInput)

	updated := config.Host{
		ID:            h.ID,
		Alias:         newAlias,
		User:          newUser,
		Host:          newHost,
		Port:          newPort,
		Identity:      newIdentity,
		Note:          newNote,
		Tags:          newTags,
		Auth:          newAuth,
		PasswordRef:   h.PasswordRef,
		Pinned:        h.Pinned,
		LastUsedAt:    h.LastUsedAt,
		HostKeyPolicy: newHostKeyPolicy,
		JumpHost:      newJumpHost,
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

	var newPassword *string
	var vault *secret.FileStore
	if h.PasswordRef != "" {
		fmt.Printf("\n当前主机已保存密码。\n")
		changePass := ui.ReadYesNo("是否修改密码？[y/N]: ")
		if changePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				return fmt.Errorf("无法访问密码存储: %w", fsErr)
			}
			password, err := readConfirmedSSHPassword()
			if err != nil {
				return fmt.Errorf("修改密码失败: %w", err)
			}
			newPassword, vault = &password, fs
			updated.PasswordRef = h.ID
		}
	} else {
		savePass := ui.ReadYesNo("\n是否保存 SSH 密码？[y/N]: ")
		if savePass {
			fs, fsErr := app.requireSecretStore()
			if fsErr != nil {
				return fmt.Errorf("无法访问密码存储: %w", fsErr)
			}
			password, err := readConfirmedSSHPassword()
			if err != nil {
				return fmt.Errorf("保存密码失败: %w", err)
			}
			newPassword, vault = &password, fs
			updated.PasswordRef = h.ID
		}
	}

	updateHost := func(doc *config.Document) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID == h.ID {
				doc.Hosts[i] = updated
				return nil
			}
		}
		return fmt.Errorf("未找到主机: %s", h.Alias)
	}
	if newPassword != nil {
		if err := vault.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
			delete(entries, "password:"+h.PasswordRef)
			entries["password:"+h.ID] = *newPassword
			return updateHost(doc)
		}); err != nil {
			return err
		}
		ui.PrintSuccess("密码已加密保存")
	} else if err := app.Store.Repository().Update(updateHost); err != nil {
		return err
	}

	ui.PrintSuccess("已更新主机：%s", newAlias)
	return nil
}

func managedIdentityDisplay(identity string) string {
	if name, ok := config.ManagedKeyName(identity); ok {
		return name
	}
	return identity
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

func readConfirmedSSHPassword() (string, error) {
	pass1, err := ui.ReadPassword("请输入 SSH 密码: ")
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	pass2, err := ui.ReadPassword("再次输入 SSH 密码: ")
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}

	if pass1 != pass2 {
		return "", fmt.Errorf("两次密码不一致")
	}
	return pass1, nil
}

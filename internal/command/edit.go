package command

import (
	"fmt"
	"strconv"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

func (app *App) cmdEdit(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入要编辑的主机别名或ID: ")
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintHeader(fmt.Sprintf("编辑主机: %s (留空保留原值，输入 - 清空可选字段)", h.Alias))
	fmt.Println()

	patch, err := readHostEditPatch(*h)
	if err != nil {
		return err
	}
	candidate := *h
	fieldsChanged := patch.apply(&candidate)
	if errs := candidate.Validate(); len(errs) > 0 {
		for _, e := range errs {
			ui.PrintError("%s", e)
		}
		return fmt.Errorf("输入校验失败")
	}
	for i, existing := range hf.Hosts {
		if i != idx && existing.Alias == candidate.Alias {
			return fmt.Errorf("别名 '%s' 已存在", candidate.Alias)
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
		}
	}

	if !fieldsChanged && newPassword == nil {
		ui.PrintWarn("未检测到修改")
		return nil
	}

	updateHost := func(doc *config.Document) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID == h.ID {
				patch.apply(&doc.Hosts[i])
				if newPassword != nil {
					doc.Hosts[i].PasswordRef = h.ID
				}
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

	ui.PrintSuccess("已更新主机：%s", candidate.Alias)
	return nil
}

type hostEditPatch struct {
	alias         *string
	user          *string
	host          *string
	port          *int
	identity      *string
	note          *string
	auth          *string
	hostKeyPolicy *string
	jumpHost      *string
	tags          *[]string
}

func (p hostEditPatch) apply(host *config.Host) bool {
	changed := false
	applyString := func(value *string, target *string) {
		if value != nil && *target != *value {
			*target = *value
			changed = true
		}
	}
	applyString(p.alias, &host.Alias)
	applyString(p.user, &host.User)
	applyString(p.host, &host.Host)
	if p.port != nil && host.Port != *p.port {
		host.Port = *p.port
		changed = true
	}
	applyString(p.identity, &host.Identity)
	applyString(p.note, &host.Note)
	applyString(p.auth, &host.Auth)
	applyString(p.hostKeyPolicy, &host.HostKeyPolicy)
	applyString(p.jumpHost, &host.JumpHost)
	if p.tags != nil && !equalStrings(host.Tags, *p.tags) {
		host.Tags = append([]string(nil), (*p.tags)...)
		changed = true
	}
	return changed
}

func readHostEditPatch(host config.Host) (hostEditPatch, error) {
	var patch hostEditPatch
	var err error
	if patch.alias, err = readRequiredHostEditValue("别名", host.Alias); err != nil {
		return patch, err
	}
	if patch.user, err = readRequiredHostEditValue("用户", host.User); err != nil {
		return patch, err
	}
	if patch.host, err = readRequiredHostEditValue("主机/IP", host.Host); err != nil {
		return patch, err
	}
	if patch.port, err = readHostEditPort(host.Port); err != nil {
		return patch, err
	}

	if value, changed := readOptionalEditableValue("托管密钥", managedIdentityDisplay(host.Identity)); changed {
		value = normalizeManagedIdentity(value)
		patch.identity = &value
	}
	if value, changed := readOptionalEditableValue("备注", host.Note); changed {
		patch.note = &value
	}
	if patch.auth, err = readRequiredHostEditValue("认证策略 (auto/key/password)", host.Auth); err != nil {
		return patch, err
	}
	if value, changed := readOptionalEditableValue("主机信任策略 (strict/accept-new/insecure，空为继承)", host.HostKeyPolicy); changed {
		patch.hostKeyPolicy = &value
	}
	if value, changed := readOptionalEditableValue("跳板机别名", host.JumpHost); changed {
		patch.jumpHost = &value
	}
	if value, changed := readOptionalEditableValue("标签", config.TagsToString(host.Tags)); changed {
		tags := config.ParseTags(value)
		patch.tags = &tags
	}
	return patch, nil
}

func readRequiredHostEditValue(label, current string) (*string, error) {
	value := ui.ReadLine(fmt.Sprintf("%s [%s]（留空保留）: ", label, current))
	if value == "" {
		return nil, nil
	}
	if value == "-" {
		return nil, fmt.Errorf("%s 不能为空", label)
	}
	return &value, nil
}

func readHostEditPort(current int) (*int, error) {
	value := ui.ReadLine(fmt.Sprintf("端口 [%d]（留空保留）: ", current))
	if value == "" {
		return nil, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("端口必须是整数")
	}
	return &port, nil
}

func managedIdentityDisplay(identity string) string {
	if name, ok := config.ManagedKeyName(identity); ok {
		return name
	}
	return identity
}

func readEditableValue(label, current string) string {
	value, _ := readOptionalEditableValue(label, current)
	return value
}

func readOptionalEditableValue(label, current string) (string, bool) {
	display := current
	if display == "" {
		display = "空"
	}
	value := ui.ReadLine(fmt.Sprintf("%s [%s]（留空保留，输入 - 清空）: ", label, display))
	if value == "" {
		return current, false
	}
	if value == "-" {
		return "", current != ""
	}
	return value, value != current
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

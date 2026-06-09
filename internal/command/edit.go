package command

import (
	"fmt"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdEdit(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入要编辑的主机别名或ID: ")
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

	updated := config.Host{
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

	// If alias changed, update password ref
	if newAlias != h.Alias && h.PasswordRef != "" {
		hf, _ = app.Store.Load()
		for i := range hf.Hosts {
			if hf.Hosts[i].Alias == newAlias {
				hf.Hosts[i].PasswordRef = newAlias
				app.Store.Save(hf)
				break
			}
		}
	}

	ui.PrintSuccess("已更新主机：%s", newAlias)
	return nil
}

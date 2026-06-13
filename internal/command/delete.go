package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
)

func (app *App) cmdDelete(args []string) error {
	h, idx, _, err := app.resolveHost(args, "请输入要删除的主机别名或ID: ")
	if err != nil {
		return err
	}

	ui.RenderHostDetail(*h, idx)

	confirm := ui.ReadLine(fmt.Sprintf("确认删除 %s? [y/N]: ", h.Alias))
	if confirm != "y" && confirm != "Y" && confirm != "yes" {
		ui.PrintWarn("已取消")
		return nil
	}

	removeHost := func(doc *config.Document) error {
		for i := range doc.Hosts {
			if doc.Hosts[i].ID == h.ID {
				doc.Hosts = append(doc.Hosts[:i], doc.Hosts[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("未找到主机: %s", h.Alias)
	}
	if h.PasswordRef != "" {
		fs, err := app.requireSecretStore()
		if err != nil {
			return fmt.Errorf("删除前解锁密码库失败，未修改配置: %w", err)
		}
		if err := fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
			delete(entries, "password:"+h.ID)
			delete(entries, "password:"+h.PasswordRef)
			return removeHost(doc)
		}); err != nil {
			return fmt.Errorf("删除主机失败，配置未修改: %w", err)
		}
	} else if err := app.Store.Repository().Update(removeHost); err != nil {
		return err
	}

	ui.PrintSuccess("已删除主机：%s", h.Alias)
	return nil
}

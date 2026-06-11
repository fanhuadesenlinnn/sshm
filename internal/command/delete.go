package command

import (
	"errors"
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/internal/keymgr"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
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

	removeKey := false
	if keymgr.IsManagedKey(h.Identity) {
		removeKey = ui.ReadYesNo("是否同时删除 sshm 管理的密钥？[y/N]: ")
	}

	if err := app.Store.Remove(idx); err != nil {
		return err
	}

	var cleanupErrs []error
	if h.PasswordRef != "" {
		fs := app.tryGetSecretStore()
		if fs == nil {
			ui.PrintWarn("主机已删除，但密码库未解锁；可能仍有残留密码引用")
		} else if err := fs.RemovePasswords(h.Alias, h.ID, h.PasswordRef); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("清理保存密码失败: %w", err))
		}
	}
	if removeKey {
		if err := keymgr.RemoveManagedKey(h.Identity); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("清理托管密钥失败: %w", err))
		}
	}
	if len(cleanupErrs) > 0 {
		return fmt.Errorf("主机已删除，但清理附属数据失败: %w", errors.Join(cleanupErrs...))
	}

	ui.PrintSuccess("已删除主机：%s", h.Alias)
	return nil
}

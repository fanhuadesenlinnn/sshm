package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdShowPubkey(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}

	name, managed := config.ManagedKeyName(h.Identity)
	if !managed {
		return fmt.Errorf("主机 %s 未配置托管密钥", h.Alias)
	}
	key, err := app.keyStore().Find(name)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println(key.PublicKey)
	fmt.Println()
	return nil
}

func (app *App) cmdAuth(args []string) error {
	h, idx, hf, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("当前认证策略:", h.Auth)
	fmt.Println()
	fmt.Println("可选: auto / key / password")
	fmt.Println()

	newAuth := ui.ReadLineDefault(fmt.Sprintf("新认证策略 [%s]: ", h.Auth), h.Auth)

	validAuth := map[string]bool{
		"auto": true, "key": true, "password": true,
	}
	if !validAuth[newAuth] {
		return fmt.Errorf("无效的认证策略: %s", newAuth)
	}

	hf.Hosts[idx].Auth = newAuth
	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("更新主机配置失败: %w", err)
	}

	ui.PrintSuccess("认证策略已更新：%s -> %s", h.Alias, newAuth)
	return nil
}

package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/keymgr"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdImportKey(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm --import-key <别名|ID> <私钥路径>")
	}

	h, idx, hf, err := app.Store.FindHost(args[0])
	if err != nil {
		return err
	}

	srcPath := args[1]
	relPath, err := keymgr.ImportKey(h.Alias, srcPath)
	if err != nil {
		return fmt.Errorf("导入密钥失败: %w", err)
	}

	hf.Hosts[idx].Identity = relPath
	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("更新主机配置失败: %w", err)
	}

	ui.PrintSuccess("密钥已导入：%s -> %s", h.Alias, relPath)
	return nil
}

func (app *App) cmdGenKey(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: sshm --gen-key <别名|ID>")
	}

	h, idx, hf, err := app.Store.FindHost(args[0])
	if err != nil {
		return err
	}

	relPath, err := keymgr.GenerateKey(h.Alias)
	if err != nil {
		return fmt.Errorf("生成密钥失败: %w", err)
	}

	hf.Hosts[idx].Identity = relPath
	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("更新主机配置失败: %w", err)
	}

	ui.PrintSuccess("密钥已生成：%s", relPath)
	fmt.Println()
	fmt.Println("  请将以下公钥放到远端服务器 ~/.ssh/authorized_keys：")
	fmt.Println()

	pubKey, _ := keymgr.ShowPubKey(hf.Hosts[idx])
	if pubKey != "" {
		fmt.Println("  " + pubKey)
	}
	fmt.Println()

	return nil
}

func (app *App) cmdShowPubkey(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入主机别名或ID: ")
	if err != nil {
		return err
	}

	if name, managed := config.ManagedKeyName(h.Identity); managed {
		key, err := app.keyStore().Find(name)
		if err != nil {
			return err
		}
		fmt.Println()
		fmt.Println(key.PublicKey)
		fmt.Println()
		return nil
	}

	pubKey, err := keymgr.ShowPubKey(*h)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(pubKey)
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
	fmt.Println("可选: auto / key / password / ask / system")
	fmt.Println()

	newAuth := ui.ReadLineDefault(fmt.Sprintf("新认证策略 [%s]: ", h.Auth), h.Auth)

	validAuth := map[string]bool{
		"auto": true, "key": true, "password": true, "ask": true, "system": true,
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

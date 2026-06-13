package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/keymgr"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
	"golang.org/x/crypto/ssh"
)

func (app *App) cmdKey(args []string) error {
	if len(args) == 0 {
		if ui.IsTerminal() {
			return app.keyCenter()
		}
		app.printKeyHelp()
		return nil
	}
	switch args[0] {
	case "list", "ls", "l":
		return app.cmdKeyList(args[1:])
	case "create", "new", "c":
		return app.cmdKeyCreate(args[1:])
	case "create-batch":
		return app.cmdKeyCreateBatch(args[1:])
	case "import", "i":
		return app.cmdKeyImport(args[1:])
	case "import-batch":
		return app.cmdKeyImportBatch(args[1:])
	case "default":
		return app.cmdKeyDefault(args[1:])
	case "show", "pub", "public":
		return app.cmdKeyShow(args[1:])
	case "use", "bind":
		return app.cmdKeyUse(args[1:])
	case "push":
		return app.cmdKeyRemote(args[1:], false)
	case "setup":
		return app.cmdKeySetup(args[1:])
	case "revoke":
		return app.cmdKeyRemote(args[1:], true)
	case "status":
		return app.cmdKeyStatus(args[1:])
	case "delete", "del", "rm":
		return app.cmdKeyDelete(args[1:])
	case "delete-unused":
		return app.cmdKeyDeleteUnused(args[1:])
	case "help", "h":
		app.printKeyHelp()
		return nil
	default:
		return fmt.Errorf("未知 key 命令 %q；使用 sshm key help 查看帮助", args[0])
	}
}

func (app *App) keyCenter() error {
	app.printKeyHelp()
	for {
		parts := parseArgs(ui.ReadLine(ui.CyanText("key> ")))
		if len(parts) == 0 {
			continue
		}
		if parts[0] == "q" || parts[0] == "quit" || parts[0] == "back" {
			return nil
		}
		if err := app.cmdKey(parts); err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

func (app *App) printKeyHelp() {
	ui.PrintHeader("托管密钥中心")
	fmt.Println()
	fmt.Println("  l/list                         列出托管密钥")
	fmt.Println("  c/create <名称> [--default]    生成并加密保存密钥")
	fmt.Println("  create-batch <名称...>         批量生成密钥")
	fmt.Println("  i/import <名称> <私钥路径>     导入并加密保存密钥")
	fmt.Println("  import-batch <名称=路径...>    批量导入密钥")
	fmt.Println("  default [名称|-]               查看、设置或取消默认密钥")
	fmt.Println("  show [名称|default]            显示公钥")
	fmt.Println("  use <密钥> <目标...>           将主机绑定到密钥")
	fmt.Println("  push <密钥> <目标...>          推送公钥到远端")
	fmt.Println("  setup <密钥> <目标...>         推送、验证并绑定")
	fmt.Println("  revoke <密钥> <目标...>        从远端撤销公钥")
	fmt.Println("  status [密钥]                  显示主机绑定状态")
	fmt.Println("  delete <名称...>               删除本地托管密钥")
	fmt.Println("  delete-unused                  删除未绑定且非默认的密钥")
	fmt.Println()
	fmt.Println("  目标支持：别名...、--tag 标签、--all")
	fmt.Println("  输入 back/q 返回主命令页")
	fmt.Println()
}

func (app *App) cmdKeyList(_ []string) error {
	kf, err := app.keyStore().Load()
	if err != nil {
		return err
	}
	if len(kf.Keys) == 0 {
		ui.PrintWarn("暂无托管密钥，使用 sshm key create <名称> 创建")
		return nil
	}
	fmt.Println()
	fmt.Printf("  %-20s %-10s %-24s %s\n", "名称", "默认", "创建时间", "指纹")
	for _, key := range kf.Keys {
		marker := ""
		if key.Name == kf.Default {
			marker = "yes"
		}
		fingerprint := publicKeyFingerprint(key.PublicKey)
		fmt.Printf("  %-20s %-10s %-24s %s\n", key.Name, marker, key.CreatedAt, fingerprint)
	}
	fmt.Println()
	return nil
}

func (app *App) cmdKeyCreate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: sshm key create <名称> [--default]")
	}
	name := args[0]
	makeDefault := containsArg(args[1:], "--default")
	if err := config.ValidateManagedKeyName(name); err != nil {
		return err
	}
	privateKey, publicKey, err := keymgr.GenerateManagedKey(name)
	if err != nil {
		return err
	}
	return app.saveManagedKey(name, privateKey, publicKey, makeDefault)
}

func (app *App) cmdKeyCreateBatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sshm key create-batch <名称...>")
	}
	failed := 0
	for _, name := range args {
		if err := app.cmdKeyCreate([]string{name}); err != nil {
			failed++
			ui.PrintError("%s: %v", name, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("批量创建完成，%d 个失败", failed)
	}
	return nil
}

func (app *App) cmdKeyImport(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm key import <名称> <私钥路径> [--default]")
	}
	name := args[0]
	if err := config.ValidateManagedKeyName(name); err != nil {
		return err
	}
	path := config.ExpandPath(args[1])
	privateKey, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取私钥 %s 失败: %w", path, err)
	}
	passphrase := []byte(nil)
	parsed, publicKey, err := keymgr.ParseManagedKey(privateKey, passphrase, name)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		value, readErr := ui.ReadPassword("请输入导入私钥的密码: ")
		if readErr != nil {
			return fmt.Errorf("读取私钥密码失败: %w", readErr)
		}
		parsed, publicKey, err = keymgr.ParseManagedKey(privateKey, []byte(value), name)
	}
	if err != nil {
		return err
	}
	return app.saveManagedKey(name, parsed, publicKey, containsArg(args[2:], "--default"))
}

func (app *App) cmdKeyImportBatch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sshm key import-batch <名称=私钥路径...>")
	}
	failed := 0
	for _, pair := range args {
		name, path, ok := strings.Cut(pair, "=")
		if !ok || name == "" || path == "" {
			failed++
			ui.PrintError("%s: 应使用 名称=路径 格式", pair)
			continue
		}
		if err := app.cmdKeyImport([]string{name, path}); err != nil {
			failed++
			ui.PrintError("%s: %v", name, err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("批量导入完成，%d 个失败", failed)
	}
	return nil
}

func (app *App) saveManagedKey(name string, privateKey []byte, publicKey string, makeDefault bool) error {
	if _, err := app.keyStore().Find(name); err == nil {
		return fmt.Errorf("托管密钥 %q 已存在，拒绝覆盖", name)
	}
	fs, err := app.requireSecretStore()
	if err != nil {
		return err
	}
	if err := fs.SetManagedKey(name, privateKey); err != nil {
		return fmt.Errorf("加密保存私钥失败: %w", err)
	}
	if err := app.keyStore().Add(name, publicKey, makeDefault); err != nil {
		_ = fs.RemoveManagedKeys(name)
		return fmt.Errorf("保存密钥元数据失败: %w", err)
	}
	ui.PrintSuccess("托管密钥已加密保存：%s", name)
	return nil
}

func (app *App) cmdKeyDefault(args []string) error {
	if len(args) == 0 {
		kf, err := app.keyStore().Load()
		if err != nil {
			return err
		}
		if kf.Default == "" {
			ui.PrintWarn("尚未设置默认托管密钥")
		} else {
			fmt.Println(kf.Default)
		}
		return nil
	}
	if args[0] == "-" {
		if err := app.keyStore().SetDefault(""); err != nil {
			return err
		}
		ui.PrintSuccess("已取消默认托管密钥")
		return nil
	}
	if err := app.keyStore().SetDefault(args[0]); err != nil {
		return err
	}
	ui.PrintSuccess("默认托管密钥已设置为：%s", args[0])
	return nil
}

func (app *App) cmdKeyDeleteUnused(_ []string) error {
	kf, err := app.keyStore().Load()
	if err != nil {
		return err
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	used := map[string]bool{kf.Default: true}
	for _, host := range hf.Hosts {
		if name, ok := config.ManagedKeyName(host.Identity); ok {
			used[name] = true
		}
	}
	var unused []string
	for _, key := range kf.Keys {
		if !used[key.Name] {
			unused = append(unused, key.Name)
		}
	}
	if len(unused) == 0 {
		ui.PrintWarn("没有可删除的未使用托管密钥")
		return nil
	}
	return app.cmdKeyDelete(unused)
}

func (app *App) cmdKeyShow(args []string) error {
	name := "default"
	if len(args) > 0 {
		name = args[0]
	}
	key, err := app.keyStore().Find(name)
	if err != nil {
		return err
	}
	fmt.Println(key.PublicKey)
	return nil
}

func (app *App) cmdKeyDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sshm key delete <名称...>")
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	for _, name := range args {
		var aliases []string
		for _, host := range hf.Hosts {
			if keyName, ok := config.ManagedKeyName(host.Identity); ok && keyName == name {
				aliases = append(aliases, host.Alias)
			}
		}
		if len(aliases) > 0 {
			return fmt.Errorf("密钥 %s 仍被主机使用: %s；请先执行 key use 重新绑定", name, strings.Join(aliases, ", "))
		}
	}
	fs, err := app.requireSecretStore()
	if err != nil {
		return fmt.Errorf("解锁密码库失败，未删除任何密钥: %w", err)
	}
	if err := app.keyStore().Remove(args...); err != nil {
		return err
	}
	if err := fs.RemoveManagedKeys(args...); err != nil {
		return fmt.Errorf("密钥元数据已删除，但清理加密私钥失败: %w", err)
	}
	ui.PrintSuccess("已删除托管密钥：%s", strings.Join(args, ", "))
	return nil
}

func publicKeyFingerprint(line string) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return "invalid"
	}
	return ssh.FingerprintSHA256(key)
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

package command

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) interactiveMode() error {
	if _, err := app.Store.Load(); err != nil {
		if errors.Is(err, config.ErrNotInitialized) {
			app.printFirstRunGuide()
			return nil
		}
		return fmt.Errorf("加载主机配置失败: %w", err)
	}
	if !ui.IsTerminal() {
		app.printHelp()
		return nil
	}
	app.printWorkbench()

	for {
		input := ui.ReadLine(ui.CyanText("sshm> "))
		if input == "" {
			continue
		}

		parts, parseErr := parseInteractiveInput(input)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", parseErr))
			continue
		}
		if len(parts) == 0 {
			continue
		}

		exit, err := app.dispatchInteractive(parts)
		if exit {
			fmt.Println("bye.")
			return nil
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

// dispatchInteractive routes one parsed interactive command line. It returns
// exit=true when the user asked to leave the workspace.
func (app *App) dispatchInteractive(parts []string) (bool, error) {
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	var err error
	switch cmd {
	case "host":
		err = app.cmdHost(args)
	case "key", "k":
		err = app.cmdKey(args)
	case "list", "ls", "l":
		err = app.cmdList(args)
	case "add", "a":
		err = app.cmdAdd(args)
	case "add-batch", "ab":
		err = app.cmdAddBatch(args)
	case "edit", "e":
		err = app.cmdEdit(args)
	case "del", "delete", "rm", "d":
		err = app.cmdDelete(args)
	case "conn", "connect", "c":
		err = app.cmdConnect(args)
	case "show", "info":
		err = app.cmdShow(args)
	case "search", "find", "s":
		err = app.cmdSearch(args)
	case "tag", "tags":
		err = app.cmdTag(args)
	case "deploy":
		err = app.cmdDeploy(args)
	case "recent", "r":
		err = app.cmdRecent(args)
	case "pin":
		err = app.cmdPin(args, true)
	case "unpin":
		err = app.cmdPin(args, false)
	case "find-con", "pick", "f":
		err = app.cmdPick(args)
	case "doctor":
		err = app.cmdDoctor(args)
	case "ping", "p":
		err = app.cmdPing(args)
	case "exec", "x":
		err = app.cmdInteractiveExec(args)
	case "exec-tag", "xt":
		err = app.cmdInteractiveExecTag(args)
	case "push":
		err = app.cmdPush(args)
	case "pull":
		err = app.cmdPull(args)
	case "forward":
		err = app.cmdForward(args)
	case "logs":
		err = app.cmdLogs(args)
	case "config":
		err = app.cmdConfig(args)
	case "config-edit":
		err = app.cmdConfigEdit(args)
	case "ssh-config", "sc":
		err = app.cmdInteractiveSSHConfig(args)
	case "passwd":
		err = app.cmdPasswd(args)
	case "forget-pass":
		err = app.cmdForgetPass(args)
	case "show-pubkey":
		err = app.cmdShowPubkey(args)
	case "auth":
		err = app.cmdAuth(args)
	case "lock":
		app.lockSecretStore()
		ui.PrintSuccess("当前会话密码库已锁定")
	case "help", "h":
		app.printInteractiveHelp()
	case "exit", "quit", "q":
		return true, nil
	default:
		err = app.cmdConnect(parts)
	}
	return false, err
}

func (app *App) printWorkbench() {
	ui.PrintHeader("sshm 工作台")
	fmt.Println()
	fmt.Println("  p/ping        测试连接")
	fmt.Println("  f/find-con    查找并连接主机")
	fmt.Println("  r/recent      收藏与最近使用")
	fmt.Println("  a/add         添加主机")
	fmt.Println("  host/key/tag  主机、密钥与标签管理")
	fmt.Println("  deploy        批量部署工作流")
	fmt.Println("  h/help        完整帮助")
	fmt.Println("  lock / q      锁定密码库 / 退出")
	fmt.Println()
	hf, err := app.Store.Load()
	if err != nil {
		return
	}
	recent := make([]config.Host, 0, 5)
	for _, host := range hf.Hosts {
		if host.Pinned || host.LastUsedAt != "" {
			recent = append(recent, host)
		}
	}
	if len(recent) > 5 {
		recent = recent[:5]
	}
	if len(recent) > 0 {
		fmt.Println("  最近与收藏:")
		for _, host := range recent {
			fmt.Printf("    %-18s %s@%s\n", host.Alias, host.User, host.Host)
		}
		fmt.Println()
	}
}

func (app *App) cmdConnect(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("请指定主机别名或ID")
	}
	if len(args) > 1 {
		return fmt.Errorf("不支持透传 OpenSSH 参数；请使用 sshm 的原生能力")
	}

	aliasOrID := args[0]

	h, _, _, err := app.findHost(aliasOrID)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			if markErr := app.Store.MarkUsed(h.ID, time.Now().Format(time.RFC3339)); markErr != nil {
				ui.PrintWarn("连接成功，但记录最近使用失败: %v", markErr)
			}
		}
	}()

	fmt.Println()
	fmt.Printf("连接 %s (%s@%s:%d)...\n", h.Alias, h.User, h.Host, h.Port)
	fmt.Println()

	fs := app.getSecretStoreForHost(h)
	err = sshx.Connect(*h, fs)
	if err == nil {
		return nil
	}
	stage := operation.StageOf(err, operation.StageSession)
	if !shouldTryTemporaryPassword(err) {
		return connectionFailure(*h, err)
	}
	fmt.Fprintln(os.Stderr, ui.Warn("已配置认证连接失败（阶段: %s）: %v", stage, err))
	if !ui.IsTerminal() {
		return connectionFailure(*h, err)
	}
	return app.promptAndConnect(*h, fs)
}

func shouldTryTemporaryPassword(err error) bool {
	stage := operation.StageOf(err, operation.StageSession)
	return stage == operation.StageAuth || stage == operation.StageCredential
}

func (app *App) promptAndConnect(h config.Host, fs *secret.FileStore) error {
	if h.JumpHost != "" && fs == nil {
		fs = app.tryGetSecretStore()
	}
	pass, err := ui.ReadPassword("请输入 SSH 密码: ")
	if err != nil {
		return fmt.Errorf("读取密码失败: %w", err)
	}

	if err := sshx.ConnectTemporaryPassword(h, fs, pass); err != nil {
		return connectionFailure(h, fmt.Errorf("临时密码认证失败: %w", err))
	}

	fmt.Println()
	if ui.ReadYesNo("是否保存此密码以供下次使用？[y/N]: ") {
		if fs == nil {
			var storeErr error
			fs, storeErr = app.requireSecretStore()
			if storeErr != nil {
				return fmt.Errorf("保存密码前解锁密码库失败: %w", storeErr)
			}
		}
		if fs != nil {
			ref := h.ID
			if err := fs.UpdateDocument(func(doc *config.Document, entries map[string]string) error {
				entries["password:"+ref] = pass
				for i := range doc.Hosts {
					if doc.Hosts[i].ID == h.ID {
						doc.Hosts[i].PasswordRef = ref
						return nil
					}
				}
				return fmt.Errorf("未找到主机: %s", h.Alias)
			}); err != nil {
				ui.PrintWarn("保存密码失败: %v", err)
			} else {
				ui.PrintSuccess("密码已加密保存：%s", h.Alias)
			}
		}
	}

	return nil
}

func connectionFailure(h config.Host, err error) error {
	stage := operation.StageOf(err, operation.StageSession)
	return fmt.Errorf("连接 %s 失败（阶段: %s）: %w；建议: %s", h.Alias, stage, err, operation.Suggestion(stage))
}

func (app *App) cmdInteractiveExec(args []string) error {
	_, _, _, rest := parseOperationFlags(args)
	if len(rest) < 2 {
		alias := ""
		if len(rest) == 1 {
			alias = rest[0]
		} else {
			alias = ui.ReadLine("目标主机 (别名/ID): ")
		}
		cmd := ui.ReadLine("命令: ")
		if alias == "" || cmd == "" {
			return fmt.Errorf("需要指定目标主机和远程命令")
		}
		prefix := append([]string(nil), args[:len(args)-len(rest)]...)
		return app.cmdExec(append(prefix, alias, "--", cmd))
	}
	return app.cmdExec(args)
}

func (app *App) cmdInteractiveExecTag(args []string) error {
	_, positionals, err := parseBatchCLIOptions(args)
	if err != nil {
		return err
	}
	if len(positionals) < 2 {
		if len(positionals) == 0 {
			tag := ui.ReadLine("标签名: ")
			if tag == "" {
				return fmt.Errorf("需要指定标签")
			}
			args = append(args, tag)
		}
		cmd := ui.ReadLine("命令: ")
		if cmd == "" {
			return fmt.Errorf("需要指定远程命令")
		}
		return app.cmdExecTag(append(args, "--", cmd))
	}
	return app.cmdExecTag(args)
}

func (app *App) cmdInteractiveSSHConfig(args []string) error {
	fmt.Println()
	fmt.Println("  1. 导出 SSH 配置")
	fmt.Println("  2. 导入 SSH 配置")
	fmt.Println()
	choice := ui.ReadLine("选择 [1/2]: ")

	switch choice {
	case "1":
		path := ui.ReadLine("导出路径（必填，留空取消）: ")
		if path == "" {
			return nil
		}
		return app.cmdExportSSHConfig([]string{path})
	case "2":
		path := ui.ReadLine("导入路径 (留空使用 ~/.ssh/config): ")
		if path != "" {
			return app.cmdImportSSHConfig([]string{path})
		}
		return app.cmdImportSSHConfig(nil)
	default:
		return nil
	}
}

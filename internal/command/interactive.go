package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

func (app *App) interactiveMode() error {
	if _, err := app.Store.Load(); err != nil {
		return fmt.Errorf("加载主机配置失败: %w", err)
	}
	app.printWorkbench()

	for {
		input := ui.ReadLine(ui.CyanText("sshm> "))
		if input == "" {
			continue
		}

		parts := parseArgs(input)
		if len(parts) == 0 {
			continue
		}

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
		case "pick", "p":
			err = app.cmdPick(args)
		case "doctor":
			err = app.cmdDoctor(args)
		case "ping":
			err = app.cmdPing(args)
		case "exec", "x":
			err = app.cmdInteractiveExec(args)
		case "exec-tag", "xt":
			err = app.cmdInteractiveExecTag(args)
		case "exec-all", "xa":
			err = app.cmdInteractiveExecAll(args)
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
			fmt.Println("bye.")
			return nil
		default:
			err = app.cmdConnect(parts)
		}

		if err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

func (app *App) printWorkbench() {
	ui.PrintHeader("sshm 工作台")
	fmt.Println()
	fmt.Println("  p/pick        查找并连接主机")
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

	h, _, _, err := app.Store.FindHost(aliasOrID)
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
	return operation.StageOf(err, operation.StageSession) == operation.StageAuth
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
	if len(args) < 2 {
		alias := ui.ReadLine("目标主机 (别名/ID): ")
		cmd := ui.ReadLine("命令: ")
		return app.cmdExec([]string{alias, cmd})
	}
	return app.cmdExec(args)
}

func (app *App) cmdInteractiveExecTag(args []string) error {
	if len(args) < 2 {
		tag := ui.ReadLine("标签名: ")
		cmd := ui.ReadLine("命令: ")
		return app.cmdExecTag([]string{tag, cmd})
	}
	return app.cmdExecTag(args)
}

func (app *App) cmdInteractiveExecAll(args []string) error {
	if len(args) < 1 {
		cmd := ui.ReadLine("命令: ")
		return app.cmdExecAll([]string{cmd})
	}
	return app.cmdExecAll(args)
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

func parseArgs(input string) []string {
	var args []string
	current := ""
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			} else {
				current += string(ch)
			}
		} else {
			switch ch {
			case '"', '\'':
				inQuote = true
				quoteChar = ch
			case ' ', '\t':
				if current != "" {
					args = append(args, current)
					current = ""
				}
			default:
				current += string(ch)
			}
		}
	}
	if current != "" {
		args = append(args, current)
	}
	return args
}

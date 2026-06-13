package command

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) interactiveMode() error {
	if _, err := app.Store.Load(); err != nil {
		return fmt.Errorf("加载主机配置失败: %w", err)
	}
	app.printInteractiveHelp()

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

func (app *App) cmdConnect(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("请指定主机别名或ID")
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

	if _, managed := config.ManagedKeyName(h.Identity); managed {
		fs, storeErr := app.requireSecretStore()
		if storeErr != nil {
			return storeErr
		}
		return sshx.Connect(*h, fs)
	}

	var fs *secret.FileStore
	if h.PasswordRef != "" {
		fs = app.tryGetSecretStore()
	}

	err = sshx.Connect(*h, fs)
	if err == nil {
		return nil
	}

	if h.PasswordRef == "" || fs == nil {
		return app.promptAndConnect(*h, fs)
	}

	return err
}

func (app *App) promptAndConnect(h config.Host, fs *secret.FileStore) error {
	pass, err := ui.ReadPassword("请输入 SSH 密码: ")
	if err != nil {
		return fmt.Errorf("读取密码失败: %w", err)
	}

	if err := sshx.NativeConnectPassword(h, pass); err != nil {
		return fmt.Errorf("密码认证失败: %w", err)
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
			if ref == "" {
				ref = h.Alias
			}
			if err := fs.SetPassword(ref, pass); err != nil {
				ui.PrintWarn("保存密码失败: %v", err)
			} else {
				hf, loadErr := app.Store.Load()
				if loadErr == nil {
					for i := range hf.Hosts {
						if hf.Hosts[i].Alias == h.Alias {
							hf.Hosts[i].PasswordRef = ref
							if hf.Hosts[i].Auth == "" {
								hf.Hosts[i].Auth = "auto"
							}
							if err := app.Store.Save(hf); err != nil {
								return fmt.Errorf("密码已保存，但更新主机配置失败: %w", err)
							}
							break
						}
					}
				}
				ui.PrintSuccess("密码已加密保存：%s", h.Alias)
			}
		}
	}

	return nil
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
		path := ui.ReadLine("导出路径 (留空使用默认): ")
		if path != "" {
			return app.cmdExportSSHConfig([]string{path})
		}
		return app.cmdExportSSHConfig(nil)
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

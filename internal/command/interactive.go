package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) interactiveMode() error {
	fmt.Println()
	ui.PrintHeader("sshm - SSH 主机管理器")
	fmt.Println()

	// Show host list at startup
	hf, err := app.Store.Load()
	if err != nil {
		return fmt.Errorf("加载主机配置失败: %w", err)
	}
	ui.RenderHostsTable(hf.Hosts)

	fmt.Printf("输入 %s 查看可用命令，输入 %s 退出\n", ui.CyanText("h"), ui.CyanText("q"))

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

		// All commands return errors that we display without exiting
		var err error
		switch cmd {
		case "list", "ls", "l":
			err = app.cmdList(args)
		case "add", "a":
			err = app.cmdAdd(args)
		case "edit", "e":
			err = app.cmdEdit(args)
		case "del", "delete", "rm", "d":
			err = app.cmdDelete(args)
		case "copy", "cp":
			err = app.cmdCopy(args)
		case "conn", "connect", "c":
			err = app.cmdConnect(args)
		case "show", "info":
			err = app.cmdShow(args)
		case "search", "find", "s":
			err = app.cmdSearch(args)
		case "group", "g":
			err = app.cmdGroup(args)
		case "ping", "p":
			err = app.cmdPing(args)
		case "exec", "x":
			err = app.cmdInteractiveExec(args)
		case "exec-group", "xg":
			err = app.cmdInteractiveExecGroup(args)
		case "exec-all", "xa":
			err = app.cmdInteractiveExecAll(args)
		case "ssh-config", "sc":
			err = app.cmdInteractiveSSHConfig(args)
		case "passwd":
			err = app.cmdPasswd(args)
		case "forget-pass":
			err = app.cmdForgetPass(args)
		case "import-key":
			err = app.cmdImportKey(args)
		case "gen-key":
			err = app.cmdGenKey(args)
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
			// Try numeric or alias connect
			err = app.cmdConnect(parts)
		}

		// Display errors but keep the shell alive
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

// cmdConnect connects to a host by alias or ID.
func (app *App) cmdConnect(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定主机别名或ID")
	}

	aliasOrID := args[0]
	extraArgs := args[1:]

	h, _, _, err := app.Store.FindHost(aliasOrID)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("连接 %s (%s@%s:%d)...\n", h.Alias, h.User, h.Host, h.Port)
	fmt.Println()

	var fs *secret.FileStore
	if h.PasswordRef != "" {
		fs = app.tryGetSecretStore()
	}

	strategy := sshx.GetAuthStrategy(h.Auth)

	// If user wants system SSH or ask-each-time, delegate directly
	if strategy == sshx.AuthSystem || strategy == sshx.AuthAsk {
		return sshx.ConnectSystem(*h, extraArgs)
	}

	// If user wants key-only auth
	if strategy == sshx.AuthKey {
		if !sshx.HasIdentity(*h) {
			return fmt.Errorf("主机 %s 未配置密钥", h.Alias)
		}
		code := sshx.ConnectOpenSSHKey(*h, extraArgs)
		if code != 0 {
			return fmt.Errorf("密钥认证失败 (exit %d)", code)
		}
		return nil
	}

	// If user wants password-only auth
	if strategy == sshx.AuthPassword {
		return app.connectWithPassword(*h, fs, extraArgs)
	}

	// AuthAuto: try key first, then password, then prompt
	hasKey := sshx.HasIdentity(*h)

	if hasKey {
		code := sshx.ConnectOpenSSHKey(*h, extraArgs)
		if code == 0 {
			return nil
		}
		fmt.Fprintf(os.Stderr, "密钥认证未通过，尝试密码连接...\n")
	}

	// Try saved password
	if h.PasswordRef != "" && fs != nil {
		pass, perr := fs.GetPassword(h.PasswordRef)
		if perr == nil {
			if len(extraArgs) > 0 {
				fmt.Fprintln(os.Stderr, "密码连接不支持额外 SSH 参数，回退到系统 SSH...")
				return sshx.ConnectSystem(*h, extraArgs)
			}
			if connErr := sshx.NativeConnectPassword(*h, pass); connErr == nil {
				return nil
			} else {
				fmt.Fprintf(os.Stderr, "密码连接失败: %v\n", connErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "读取保存密码失败: %v\n", perr)
		}
	}

	// Prompt for SSH password interactively
	return app.promptAndConnect(*h, fs, extraArgs)
}

// connectWithPassword handles AuthPassword strategy.
func (app *App) connectWithPassword(h config.Host, fs *secret.FileStore, extraArgs []string) error {
	if len(extraArgs) > 0 {
		return fmt.Errorf("password 认证不支持额外 SSH 参数；请改用 system/ask 认证策略")
	}
	if h.PasswordRef != "" && fs != nil {
		pass, err := fs.GetPassword(h.PasswordRef)
		if err == nil {
			return sshx.NativeConnectPassword(h, pass)
		}
		fmt.Fprintf(os.Stderr, "读取密码失败: %v\n", err)
	}
	return app.promptAndConnect(h, fs, extraArgs)
}

// promptAndConnect prompts for SSH password, connects, and offers to save.
func (app *App) promptAndConnect(h config.Host, fs *secret.FileStore, extraArgs []string) error {
	if len(extraArgs) > 0 {
		return fmt.Errorf("密码连接不支持额外 SSH 参数；请改用 system/ask 认证策略")
	}
	pass, err := ui.ReadPassword("请输入 SSH 密码: ")
	if err != nil {
		ui.PrintWarn("读取密码失败，回退到系统 SSH")
		return sshx.ConnectSystem(h, extraArgs)
	}

	err = sshx.NativeConnectPassword(h, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "密码连接失败: %v，回退到系统 SSH\n", err)
		return sshx.ConnectSystem(h, extraArgs)
	}

	// Connection successful — ask whether to save the password
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
			// Use stable ID as the key
			ref := h.ID
			if ref == "" {
				ref = h.Alias
			}
			if err := fs.SetPassword(ref, pass); err != nil {
				ui.PrintWarn("保存密码失败: %v", err)
			} else {
				// Update host config with stable ID as password ref
				hf, loadErr := app.Store.Load()
				if loadErr == nil {
					for i := range hf.Hosts {
						if hf.Hosts[i].Alias == h.Alias {
							hf.Hosts[i].PasswordRef = ref
							if hf.Hosts[i].Auth == "" || hf.Hosts[i].Auth == "auto" {
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

// Interactive variants for exec commands in interactive mode.
func (app *App) cmdInteractiveExec(args []string) error {
	if len(args) < 2 {
		alias := ui.ReadLine("目标主机 (别名/ID): ")
		cmd := ui.ReadLine("命令: ")
		return app.cmdExec([]string{alias, cmd})
	}
	return app.cmdExec(args)
}

func (app *App) cmdInteractiveExecGroup(args []string) error {
	if len(args) < 2 {
		group := ui.ReadLine("分组名: ")
		cmd := ui.ReadLine("命令: ")
		return app.cmdExecGroup([]string{group, cmd})
	}
	return app.cmdExecGroup(args)
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

// parseArgs splits a command line string into arguments.
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

package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/secret"
	"github.com/sshm/sshm/internal/sshx"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) interactiveMode() error {
	fmt.Println()
	ui.PrintHeader("sshm - SSH 主机管理器")
	fmt.Println()

	// Show host list at startup
	hf, _ := app.Store.Load()
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

		switch cmd {
		case "list", "ls", "l":
			app.cmdList(args)
		case "add", "a":
			app.cmdAdd(args)
		case "edit", "e":
			app.cmdEdit(args)
		case "del", "delete", "rm", "d":
			app.cmdDelete(args)
		case "copy", "cp":
			app.cmdCopy(args)
		case "conn", "connect", "c":
			app.cmdConnect(args)
		case "show", "info":
			app.cmdShow(args)
		case "search", "find", "s":
			app.cmdSearch(args)
		case "group", "g":
			app.cmdGroup(args)
		case "ping", "p":
			app.cmdPing(args)
		case "exec", "x":
			app.cmdInteractiveExec(args)
		case "exec-group", "xg":
			app.cmdInteractiveExecGroup(args)
		case "exec-all", "xa":
			app.cmdInteractiveExecAll(args)
		case "ssh-config", "sc":
			app.cmdInteractiveSSHConfig(args)
		case "passwd":
			app.cmdPasswd(args)
		case "forget-pass":
			app.cmdForgetPass(args)
		case "import-key":
			app.cmdImportKey(args)
		case "gen-key":
			app.cmdGenKey(args)
		case "show-pubkey":
			app.cmdShowPubkey(args)
		case "auth":
			app.cmdAuth(args)
		case "help", "h":
			app.printInteractiveHelp()
		case "exit", "quit", "q":
			fmt.Println("bye.")
			return nil
		default:
			// Try numeric or alias connect
			app.cmdConnect(parts)
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

	h, idx, hf, err := app.Store.FindHost(aliasOrID)
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
		return app.connectWithPassword(*h, fs, idx, hf, extraArgs)
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
		pass, err := fs.GetPassword(h.PasswordRef)
		if err == nil {
			if err := sshx.NativeConnectPassword(*h, pass); err == nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "密码连接失败: %v\n", err)
		}
	}

	// Prompt for SSH password interactively
	return app.promptAndConnect(*h, fs, idx, hf, extraArgs)
}

// connectWithPassword handles AuthPassword strategy.
func (app *App) connectWithPassword(h config.Host, fs *secret.FileStore, idx int, hf *config.HostsFile, extraArgs []string) error {
	if h.PasswordRef != "" && fs != nil {
		pass, err := fs.GetPassword(h.PasswordRef)
		if err == nil {
			return sshx.NativeConnectPassword(h, pass)
		}
		fmt.Fprintf(os.Stderr, "读取密码失败: %v\n", err)
	}
	return app.promptAndConnect(h, fs, idx, hf, extraArgs)
}

// promptAndConnect prompts for SSH password, connects, and offers to save.
func (app *App) promptAndConnect(h config.Host, fs *secret.FileStore, idx int, hf *config.HostsFile, extraArgs []string) error {
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
			fs = app.tryGetSecretStore()
		}
		if fs != nil {
			if err := fs.SetPassword(h.Alias, pass); err != nil {
				ui.PrintWarn("保存密码失败: %v", err)
			} else {
				// Update host config
				hf, loadErr := app.Store.Load()
				if loadErr == nil {
					for i := range hf.Hosts {
						if hf.Hosts[i].Alias == h.Alias {
							hf.Hosts[i].PasswordRef = h.Alias
							if hf.Hosts[i].Auth == "" || hf.Hosts[i].Auth == "auto" {
								hf.Hosts[i].Auth = "auto"
							}
							app.Store.Save(hf)
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

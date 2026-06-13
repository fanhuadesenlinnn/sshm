package command

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/keymgr"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdAdd(args []string) error {
	if len(args) > 0 {
		return app.cmdQuickAdd(args)
	}
	return app.cmdAddWizard()
}

func (app *App) cmdQuickAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm add <别名> <用户@主机[:端口]> [--identity 路径] [--group 分组] [--tags 标签]")
	}

	h := config.DefaultHost()
	h.Alias = args[0]
	var err error
	h.User, h.Host, h.Port, err = parseSSHTarget(args[1])
	if err != nil {
		return err
	}

	for i := 2; i < len(args); i++ {
		if i+1 >= len(args) {
			return fmt.Errorf("选项 %s 缺少值", args[i])
		}
		value := args[i+1]
		switch args[i] {
		case "--identity", "-i":
			h.Identity = value
		case "--group", "-g":
			h.Group = value
		case "--tags":
			h.Tags = config.ParseTags(value)
		case "--note":
			h.Note = value
		case "--auth":
			h.Auth = value
		default:
			return unknownOptionError(args[i])
		}
		i++
	}

	if err := validateHost(h); err != nil {
		return err
	}
	if err := app.Store.Add(h); err != nil {
		return fmt.Errorf("添加主机失败: %w", err)
	}
	printAddedHost(h, false)
	return nil
}

func parseSSHTarget(target string) (user, host string, port int, err error) {
	user = "root"
	port = 22
	address := target
	if at := strings.LastIndex(target, "@"); at >= 0 {
		user = target[:at]
		address = target[at+1:]
	}
	if user == "" || address == "" {
		return "", "", 0, fmt.Errorf("目标格式无效 %q，应为 用户@主机[:端口]", target)
	}

	if parsedHost, parsedPort, splitErr := net.SplitHostPort(address); splitErr == nil {
		host = parsedHost
		port, err = strconv.Atoi(parsedPort)
		if err != nil {
			return "", "", 0, fmt.Errorf("端口无效 %q", parsedPort)
		}
	} else if strings.Count(address, ":") == 1 {
		parts := strings.SplitN(address, ":", 2)
		if parsedPort, parseErr := strconv.Atoi(parts[1]); parseErr == nil {
			host = parts[0]
			port = parsedPort
		} else {
			return "", "", 0, fmt.Errorf("端口无效 %q", parts[1])
		}
	} else {
		host = strings.Trim(address, "[]")
	}
	return user, host, port, nil
}

func (app *App) cmdAddWizard() error {
	fmt.Println()
	ui.PrintHeader("添加新的 SSH 主机")
	fmt.Println()

	h := config.DefaultHost()
	h.Alias = ui.ReadLine("别名: ")
	if h.Alias == "" {
		return fmt.Errorf("别名不能为空")
	}
	h.User = ui.ReadLineDefault("用户 [root]: ", "root")
	h.Host = ui.ReadLine("主机/IP: ")
	if h.Host == "" {
		return fmt.Errorf("主机/IP 不能为空")
	}

	portStr := ui.ReadLineDefault("端口 [22]: ", "22")
	if port, err := strconv.Atoi(portStr); err == nil {
		h.Port = port
	}

	fmt.Println()
	fmt.Println("  认证方式: auto（推荐）/ key / password / system")
	h.Auth = ui.ReadLineDefault("认证方式 [auto]: ", "auto")

	h.Identity = ui.ReadLine("密钥路径，可留空: ")
	if h.Identity != "" && ui.ReadYesNo("是否导入密钥到 sshm keys 目录？[y/N]: ") {
		relPath, err := keymgr.ImportKey(h.Alias, h.Identity)
		if err != nil {
			ui.PrintWarn("导入密钥失败: %v", err)
		} else {
			h.Identity = relPath
		}
	}

	var sshPassword string
	savePass := false
	if shouldPromptForPassword(h) {
		if h.Auth == "auto" && h.Identity == "" {
			fmt.Println("未填写密钥，将继续设置 SSH 密码（可留空跳过）。")
		}
		var err error
		sshPassword, err = readConfirmedPassword()
		if err != nil {
			return err
		}
		savePass = sshPassword != "" && ui.ReadYesNo("是否保存此密码？[y/N]: ")
	}

	h.Note = ui.ReadLine("备注，可留空: ")
	h.Group = ui.ReadLine("分组，可留空: ")
	h.Tags = config.ParseTags(ui.ReadLine("标签（空格或逗号分隔），可留空: "))

	if err := validateHost(h); err != nil {
		return err
	}

	var fs *secret.FileStore
	var err error
	if savePass {
		fs, err = app.requireSecretStore()
		if err != nil {
			return fmt.Errorf("无法访问密码存储: %w", err)
		}
		if err := fs.SetPasswordByID(h.ID, h.Alias, sshPassword); err != nil {
			return fmt.Errorf("保存密码失败: %w", err)
		}
		h.PasswordRef = h.ID
	}

	if err := app.Store.Add(h); err != nil {
		if fs != nil {
			if cleanupErr := fs.RemovePasswords(h.ID, h.Alias); cleanupErr != nil {
				return fmt.Errorf("添加主机失败: %v；回滚密码失败: %w", err, cleanupErr)
			}
		}
		return fmt.Errorf("添加主机失败: %w", err)
	}
	printAddedHost(h, savePass)
	return nil
}

func shouldPromptForPassword(h config.Host) bool {
	return h.Auth == "password" || (h.Auth == "auto" && h.Identity == "")
}

func readConfirmedPassword() (string, error) {
	pass1, err := ui.ReadPassword("请输入 SSH 密码（留空跳过）: ")
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	if pass1 == "" {
		return "", nil
	}
	pass2, err := ui.ReadPassword("再次输入 SSH 密码: ")
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	if pass1 != pass2 {
		return "", fmt.Errorf("两次密码不一致")
	}
	return pass1, nil
}

func validateHost(h config.Host) error {
	if errs := h.Validate(); len(errs) > 0 {
		return fmt.Errorf("输入校验失败: %s", strings.Join(errs, "；"))
	}
	return nil
}

func printAddedHost(h config.Host, savedPassword bool) {
	fmt.Println()
	ui.PrintSuccess("已添加主机：%s", h.Alias)
	fmt.Printf("  %s\n", ui.Info("连接", fmt.Sprintf("%s@%s:%d", h.User, h.Host, h.Port)))
	fmt.Printf("  %s\n", ui.Info("认证", h.Auth))
	if h.Identity != "" {
		fmt.Printf("  %s\n", ui.Info("密钥", h.Identity))
	}
	if savedPassword {
		fmt.Printf("  %s\n", ui.Info("密码", "已加密保存"))
	}
	fmt.Println()
}

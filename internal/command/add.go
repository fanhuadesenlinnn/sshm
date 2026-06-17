package command

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdAdd(args []string) error {
	if len(args) > 0 {
		return app.cmdQuickAdd(args)
	}
	return app.cmdAddWizard()
}

func (app *App) cmdQuickAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm add <别名> <用户@主机[:端口]> [--identity 托管密钥] [--tags 标签] [--auth 策略] [--host-key-policy 策略] [--jump-host 别名]")
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
			h.Identity = normalizeManagedIdentity(value)
		case "--tags":
			h.Tags = config.ParseTags(value)
		case "--note":
			h.Note = value
		case "--auth":
			h.Auth = value
		case "--host-key-policy":
			h.HostKeyPolicy = value
		case "--jump-host":
			h.JumpHost = value
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
	fmt.Println("  认证方式: auto（推荐）/ key / password")
	h.Auth = ui.ReadLineDefault("认证方式 [auto]: ", "auto")

	h.Identity = normalizeManagedIdentity(ui.ReadLine("托管密钥名称，可留空（使用 sshm key create 创建）: "))

	h.Note = ui.ReadLine("备注，可留空: ")
	h.Tags = config.ParseTags(ui.ReadLine("标签（空格或逗号分隔），可留空: "))

	if err := validateHost(h); err != nil {
		return err
	}

	if err := app.Store.Add(h); err != nil {
		return fmt.Errorf("添加主机失败: %w", err)
	}
	printAddedHost(h, false)
	return nil
}

func normalizeManagedIdentity(value string) string {
	if value == "" {
		return ""
	}
	if _, ok := config.ManagedKeyName(value); ok {
		return value
	}
	return config.ManagedIdentity(value)
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
	fmt.Println("  下一步:")
	fmt.Printf("    sshm ping %s\n", h.Alias)
	if !savedPassword && h.Identity == "" {
		fmt.Printf("    sshm passwd %s\n", h.Alias)
	}
	fmt.Printf("    sshm %s\n", h.Alias)
	fmt.Println()
}

package command

import (
	"fmt"
	"os"

	"github.com/sshm/sshm/internal/config"
	"github.com/sshm/sshm/internal/secret"
	"github.com/sshm/sshm/internal/ui"
)

// App holds shared state for all commands.
type App struct {
	Store      *config.Store
	SecretPath string
}

// NewApp creates a new App with the default store.
func NewApp() *App {
	return &App{
		Store:      config.NewStore(),
		SecretPath: config.SecretsFilePath(),
	}
}

// Run parses args and dispatches to the appropriate command.
func Run(args []string) error {
	if err := config.EnsureDirs(); err != nil {
		return fmt.Errorf("初始化配置目录失败: %w", err)
	}

	app := NewApp()

	if len(args) == 0 {
		return app.interactiveMode()
	}

	switch args[0] {
	case "--list", "-l":
		return app.cmdList(args[1:])
	case "--add", "-a":
		return app.cmdAdd(args[1:])
	case "--edit", "-e":
		return app.cmdEdit(args[1:])
	case "--delete", "-d", "--del":
		return app.cmdDelete(args[1:])
	case "--copy", "--cp":
		return app.cmdCopy(args[1:])
	case "--show":
		return app.cmdShow(args[1:])
	case "--search", "-s", "--find":
		return app.cmdSearch(args[1:])
	case "--group", "-g":
		return app.cmdGroup(args[1:])
	case "--ping", "-p":
		return app.cmdPing(args[1:])
	case "--exec", "-x":
		return app.cmdExec(args[1:])
	case "--exec-group", "--xg":
		return app.cmdExecGroup(args[1:])
	case "--exec-all", "--xa":
		return app.cmdExecAll(args[1:])
	case "--export-ssh-config":
		return app.cmdExportSSHConfig(args[1:])
	case "--import-ssh-config":
		return app.cmdImportSSHConfig(args[1:])
	case "--passwd":
		return app.cmdPasswd(args[1:])
	case "--forget-pass":
		return app.cmdForgetPass(args[1:])
	case "--import-key":
		return app.cmdImportKey(args[1:])
	case "--gen-key":
		return app.cmdGenKey(args[1:])
	case "--show-pubkey":
		return app.cmdShowPubkey(args[1:])
	case "--auth":
		return app.cmdAuth(args[1:])
	case "--help", "-h", "help":
		app.printHelp()
		return nil
	case "--version", "-v":
		fmt.Println("sshm v1.0.0")
		return nil
	default:
		// Treat as alias or ID to connect
		return app.cmdConnect(args)
	}
}

func (app *App) printHelp() {
	ui.PrintHeader("sshm - SSH 主机管理器")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  sshm                          进入交互模式")
	fmt.Println("  sshm <别名|ID> [SSH参数...]    连接到主机")
	fmt.Println()
	fmt.Println("命令行参数:")
	fmt.Println("  --list, -l                    列出所有主机")
	fmt.Println("  --add, -a                     添加主机")
	fmt.Println("  --edit, -e <别名|ID>          编辑主机")
	fmt.Println("  --delete, -d <别名|ID>        删除主机")
	fmt.Println("  --copy, --cp <别名|ID>        复制连接信息")
	fmt.Println("  --show <别名|ID>              显示主机详情")
	fmt.Println("  --search, -s <关键词>         搜索主机")
	fmt.Println("  --group, -g [分组名]          列出分组")
	fmt.Println("  --ping, -p [别名|ID]          测试连接")
	fmt.Println("  --exec, -x <目标> <命令>       执行命令")
	fmt.Println("  --exec-group, --xg <分组> <命令> 分组执行命令")
	fmt.Println("  --exec-all, --xa <命令>        所有主机执行命令")
	fmt.Println("  --passwd <别名|ID>            设置 SSH 密码")
	fmt.Println("  --forget-pass <别名|ID>       删除 SSH 密码")
	fmt.Println("  --import-key <别名|ID> <路径>  导入密钥")
	fmt.Println("  --gen-key <别名|ID>           生成密钥")
	fmt.Println("  --show-pubkey <别名|ID>       显示公钥")
	fmt.Println("  --auth <别名|ID>              修改认证策略")
	fmt.Println("  --export-ssh-config [文件]    导出 SSH 配置")
	fmt.Println("  --import-ssh-config [文件]    导入 SSH 配置")
	fmt.Println("  --help, -h                    显示帮助")
	fmt.Println("  --version, -v                 显示版本")
	fmt.Println()
}

// printInteractiveHelp shows help for interactive mode with short aliases.
func (app *App) printInteractiveHelp() {
	ui.PrintHeader("sshm - 交互模式帮助")
	fmt.Println()
	fmt.Println("  进入交互模式后，可直接输入以下命令：")
	fmt.Println()
	fmt.Println("  主机管理")
	fmt.Printf("    %-14s %-24s %s\n", "list, ls, l", "列出所有主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "add, a", "添加主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "edit, e", "编辑主机", "edit <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "del, delete, rm, d", "删除主机", "del <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "show, info", "显示主机详情", "show <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "search, find, s", "搜索主机", "search <关键词>")
	fmt.Printf("    %-14s %-24s %s\n", "group, g", "分组管理", "group [分组名]")
	fmt.Printf("    %-14s %-24s %s\n", "copy, cp", "复制连接信息", "copy <别名|ID>")
	fmt.Println()
	fmt.Println("  连接与执行")
	fmt.Printf("    %-14s %-24s %s\n", "conn, connect, c", "连接到主机", "c <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "ping, p", "测试连通性", "ping [别名|ID]")
	fmt.Printf("    %-14s %-24s %s\n", "exec, x", "远程执行命令", "x <别名|ID> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "exec-group, xg", "分组执行命令", "xg <分组> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "exec-all, xa", "所有主机执行", "xa <命令>")
	fmt.Println()
	fmt.Println("  认证与密钥")
	fmt.Printf("    %-14s %-24s %s\n", "passwd", "设置 SSH 密码", "passwd <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "forget-pass", "删除已存密码", "forget-pass <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "import-key", "导入 SSH 密钥", "import-key <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "gen-key", "生成 SSH 密钥", "gen-key <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "show-pubkey", "显示公钥", "show-pubkey <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "auth", "修改认证策略", "auth <别名|ID>")
	fmt.Println()
	fmt.Println("  配置")
	fmt.Printf("    %-14s %-24s %s\n", "ssh-config, sc", "导入/导出 SSH 配置", "")
	fmt.Println()
	fmt.Println("  其他")
	fmt.Printf("    %-14s %-24s %s\n", "help, h", "显示此帮助", "")
	fmt.Printf("    %-14s %-24s %s\n", "exit, quit, q", "退出程序", "")
	fmt.Println()
	fmt.Printf("  也可以直接输入 %s 快速连接主机。\n", ui.CyanText("别名 或 ID"))
	fmt.Println()
}

// requireSecretStore creates a FileStore, prompting for master password.
func (app *App) requireSecretStore() (*secret.FileStore, error) {
	pass, err := ui.ReadPassword("请输入 sshm 主密码: ")
	if err != nil {
		return nil, fmt.Errorf("读取密码失败: %w", err)
	}
	fs := secret.NewFileStore(app.SecretPath, pass)
	return fs, nil
}

// mustGetSecretStore creates a FileStore, exiting on error.
func (app *App) mustGetSecretStore() *secret.FileStore {
	fs, err := app.requireSecretStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		os.Exit(1)
	}
	return fs
}

// resolveHost finds a host by alias or ID from args, or prompts interactively.
func (app *App) resolveHost(args []string, promptMsg string) (*config.Host, int, *config.HostsFile, error) {
	if len(args) > 0 {
		return app.Store.FindHost(args[0])
	}
	// Interactive prompt
	alias := ui.ReadLine(promptMsg)
	if alias == "" {
		return nil, -1, nil, fmt.Errorf("已取消")
	}
	return app.Store.FindHost(alias)
}

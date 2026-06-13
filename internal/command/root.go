package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
)

var Version = "dev"

// App holds shared state for all commands.
type App struct {
	Store       *config.Store
	Keys        *config.KeyStore
	ConfigPath  string
	secretStore *secret.FileStore
}

// NewApp creates a new App with the default store.
func NewApp() *App {
	return &App{
		Store:      config.NewStore(),
		Keys:       config.NewKeyStore(),
		ConfigPath: config.ConfigFilePath(),
	}
}

func (app *App) keyStore() *config.KeyStore {
	if app.Keys == nil {
		app.Keys = config.NewKeyStore()
	}
	return app.Keys
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
	case "host":
		return app.cmdHost(args[1:])
	case "key", "k":
		return app.cmdKey(args[1:])
	case "--list", "-l", "list", "ls":
		return app.cmdList(args[1:])
	case "--add", "-a", "add":
		return app.cmdAdd(args[1:])
	case "add-batch":
		return app.cmdAddBatch(args[1:])
	case "--edit", "-e", "edit":
		return app.cmdEdit(args[1:])
	case "--delete", "-d", "--del", "delete", "del", "rm":
		return app.cmdDelete(args[1:])
	case "--show", "show", "info":
		return app.cmdShow(args[1:])
	case "--search", "-s", "--find", "search", "find":
		return app.cmdSearch(args[1:])
	case "tag", "tags":
		return app.cmdTag(args[1:])
	case "recent":
		return app.cmdRecent(args[1:])
	case "pin":
		return app.cmdPin(args[1:], true)
	case "unpin":
		return app.cmdPin(args[1:], false)
	case "completion":
		return app.cmdCompletion(args[1:])
	case "pick":
		return app.cmdPick(args[1:])
	case "config-edit":
		return app.cmdConfigEdit(args[1:])
	case "config":
		return app.cmdConfig(args[1:])
	case "doctor":
		return app.cmdDoctor(args[1:])
	case "connect", "conn":
		return app.cmdConnect(args[1:])
	case "--ping", "-p", "ping":
		return app.cmdPing(args[1:])
	case "--exec", "-x", "exec":
		return app.cmdExec(args[1:])
	case "--exec-tag", "--xt", "exec-tag":
		return app.cmdExecTag(args[1:])
	case "--exec-all", "--xa", "exec-all":
		return app.cmdExecAll(args[1:])
	case "push":
		return app.cmdPush(args[1:])
	case "pull":
		return app.cmdPull(args[1:])
	case "forward":
		return app.cmdForward(args[1:])
	case "logs":
		return app.cmdLogs(args[1:])
	case "--export-ssh-config", "export-ssh-config":
		return app.cmdExportSSHConfig(args[1:])
	case "--import-ssh-config", "import-ssh-config":
		return app.cmdImportSSHConfig(args[1:])
	case "--passwd", "passwd":
		return app.cmdPasswd(args[1:])
	case "--forget-pass", "forget-pass":
		return app.cmdForgetPass(args[1:])
	case "--show-pubkey", "show-pubkey":
		return app.cmdShowPubkey(args[1:])
	case "--auth", "auth":
		return app.cmdAuth(args[1:])
	case "--lock", "lock":
		app.lockSecretStore()
		ui.PrintSuccess("当前会话密码库已锁定")
		return nil
	case "--help", "-h", "help":
		if len(args) > 1 && args[1] == "config" {
			printConfigHelp()
			return nil
		}
		app.printHelp()
		return nil
	case "--version", "-v":
		fmt.Printf("sshm %s\n", Version)
		return nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return unknownOptionError(args[0])
		}
		// Treat as alias or ID to connect
		return app.cmdConnect(args)
	}
}

func unknownOptionError(option string) error {
	commands := append([]string{}, completionCommands...)
	commands = append(commands,
		"--list", "--add", "--edit", "--delete", "--show", "--search",
		"--tag", "--ping", "--exec", "--exec-tag", "--exec-all", "--passwd",
		"--forget-pass", "--show-pubkey", "--auth",
		"--lock", "--export-ssh-config", "--import-ssh-config", "--help", "--version",
	)
	best := ""
	bestDistance := 4
	for _, command := range commands {
		if distance := editDistance(option, command); distance < bestDistance {
			best = command
			bestDistance = distance
		}
	}
	if best != "" {
		return fmt.Errorf("未知选项 %q；你是否想使用 %q？", option, best)
	}
	return fmt.Errorf("未知选项 %q；使用 sshm help 查看可用命令", option)
}

func editDistance(a, b string) int {
	aRunes := []rune(a)
	bRunes := []rune(b)
	prev := make([]int, len(bRunes)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ar := range aRunes {
		current := make([]int, len(bRunes)+1)
		current[0] = i + 1
		for j, br := range bRunes {
			cost := 0
			if ar != br {
				cost = 1
			}
			current[j+1] = min(current[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = current
	}
	return prev[len(bRunes)]
}

func (app *App) printHelp() {
	ui.PrintHeader("sshm - SSH 主机管理器")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  sshm                          进入交互模式")
	fmt.Println("  sshm <别名|ID>                 连接到主机")
	fmt.Println("  sshm <命令> [参数...]           执行管理命令")
	fmt.Println()
	fmt.Println("常用命令（旧版 --参数仍然兼容）:")
	fmt.Println("  list                          列出所有主机")
	fmt.Println("  host                          进入主机管理中心")
	fmt.Println("  key                           进入托管密钥中心")
	fmt.Println("  add [别名 user@主机[:端口]]    添加主机")
	fmt.Println("  add-batch [别名=目标...]        批量添加主机")
	fmt.Println("  edit <别名|ID>                编辑主机")
	fmt.Println("  delete <别名|ID>              删除主机")
	fmt.Println("  show <别名|ID>                显示主机详情")
	fmt.Println("  search <关键词...>            搜索主机")
	fmt.Println("  tag [标签名]                  列出标签")
	fmt.Println("  recent [数量]                 显示收藏和最近连接")
	fmt.Println("  pin/unpin <别名|ID>           收藏或取消收藏")
	fmt.Println("  completion <bash|zsh|fish>     生成 Shell 自动补全脚本")
	fmt.Println("  pick                          打开可搜索主机选择器")
	fmt.Println("  connect <别名|ID>              显式连接主机")
	fmt.Println("  doctor                        检查配置与凭据环境")
	fmt.Println("  ping [--yes] [--quiet] [目标]  测试连接")
	fmt.Println("  exec [--yes] [--quiet] ...     执行命令")
	fmt.Println("  exec-tag [--yes] [--quiet] ... 按标签执行命令")
	fmt.Println("  exec-all [--yes] [--quiet] ... 所有主机执行命令")
	fmt.Println("  push/pull ...                  安全传输文件或目录，可选 rsync 加速")
	fmt.Println("  forward <主机> <本地> <远程>   建立本地端口转发")
	fmt.Println("  logs [clean]                   查看或清理操作日志")
	fmt.Println("  passwd <别名|ID>              设置 SSH 密码")
	fmt.Println("  forget-pass <别名|ID>         删除 SSH 密码")
	fmt.Println("  show-pubkey <别名|ID>         显示公钥")
	fmt.Println("  auth <别名|ID>                修改认证策略")
	fmt.Println("  lock                           锁定当前会话密码库")
	fmt.Println("  config-edit                    校验后编辑 sshm.yaml")
	fmt.Println("  config                         查看或修改全局配置")
	fmt.Println("  export-ssh-config <文件>      导出 SSH 配置")
	fmt.Println("  import-ssh-config [文件]      导入 SSH 配置")
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
	fmt.Printf("    %-14s %-24s %s\n", "host", "进入主机管理中心", "")
	fmt.Printf("    %-14s %-24s %s\n", "list, ls, l", "列出所有主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "add, a", "添加主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "add-batch, ab", "批量添加主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "edit, e", "编辑主机", "edit <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "del, delete, rm, d", "删除主机", "del <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "show, info", "显示主机详情", "show <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "search, find, s", "搜索主机", "search <关键词>")
	fmt.Printf("    %-14s %-24s %s\n", "tag, tags", "标签管理", "tag [标签名]")
	fmt.Printf("    %-14s %-24s %s\n", "recent, r", "收藏与最近连接", "")
	fmt.Printf("    %-14s %-24s %s\n", "pin, unpin", "收藏或取消收藏", "pin <别名|ID>")
	fmt.Println()
	fmt.Println("  连接与执行")
	fmt.Printf("    %-14s %-24s %s\n", "conn, connect, c", "连接到主机", "c <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "pick, p", "打开可搜索主机选择器", "")
	fmt.Printf("    %-14s %-24s %s\n", "ping", "测试连通性", "ping [别名|ID]")
	fmt.Printf("    %-14s %-24s %s\n", "exec, x", "远程执行命令", "x <别名|ID> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "exec-tag, xt", "按标签执行命令", "xt <标签> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "exec-all, xa", "所有主机执行", "xa <命令>")
	fmt.Println()
	fmt.Println("  认证与密钥")
	fmt.Printf("    %-14s %-24s %s\n", "key, k", "进入托管密钥中心", "")
	fmt.Printf("    %-14s %-24s %s\n", "passwd", "设置 SSH 密码", "passwd <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "forget-pass", "删除已存密码", "forget-pass <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "show-pubkey", "显示公钥", "show-pubkey <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "auth", "修改认证策略", "auth <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "lock", "锁定当前会话密码库", "")
	fmt.Println()
	fmt.Println("  配置")
	fmt.Printf("    %-14s %-24s %s\n", "ssh-config, sc", "导入/导出 SSH 配置", "")
	fmt.Printf("    %-14s %-24s %s\n", "config-edit", "校验后编辑 sshm.yaml", "")
	fmt.Printf("    %-14s %-24s %s\n", "doctor", "检查本机环境", "")
	fmt.Println()
	fmt.Println("  其他")
	fmt.Printf("    %-14s %-24s %s\n", "help, h", "显示此帮助", "")
	fmt.Printf("    %-14s %-24s %s\n", "exit, quit, q", "退出程序", "")
	fmt.Println()
	fmt.Printf("  也可以直接输入 %s 快速连接主机。\n", ui.CyanText("别名 或 ID"))
	fmt.Println()
}

// requireSecretStore opens the vault, prompting for the master password.
// If a vault already exists, it verifies the passphrase before proceeding.
func (app *App) requireSecretStore() (*secret.FileStore, error) {
	if app.secretStore != nil {
		return app.secretStore, nil
	}

	exists, err := secret.VaultExists(app.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("检查 vault 失败: %w", err)
	}
	if !exists {
		fmt.Println("首次创建密码库。主密码无法恢复，请妥善保管。")
		pass1, err := ui.ReadPassword("请创建 sshm 主密码: ")
		if err != nil {
			return nil, fmt.Errorf("读取主密码失败: %w", err)
		}
		if pass1 == "" {
			return nil, fmt.Errorf("主密码不能为空")
		}
		pass2, err := ui.ReadPassword("请再次输入 sshm 主密码: ")
		if err != nil {
			return nil, fmt.Errorf("读取主密码失败: %w", err)
		}
		if pass1 != pass2 {
			return nil, fmt.Errorf("两次主密码不一致")
		}
		app.secretStore = secret.NewFileStore(app.ConfigPath, pass1)
		return app.secretStore, nil
	}

	for attempt := 1; attempt <= 3; attempt++ {
		pass, err := ui.ReadPassword("请输入 sshm 主密码: ")
		if err != nil {
			return nil, fmt.Errorf("读取主密码失败: %w", err)
		}
		fs := secret.NewFileStore(app.ConfigPath, pass)
		if err := fs.VerifyPassphrase(); err == nil {
			app.secretStore = fs
			return app.secretStore, nil
		} else if !errors.Is(err, secret.ErrIncorrectPassphrase) {
			return nil, fmt.Errorf("无法读取 vault: %w", err)
		} else if attempt < 3 {
			fmt.Fprintln(os.Stderr, ui.Warn("主密码错误，请重试 (%d/3)", attempt))
		}
	}
	return nil, secret.ErrIncorrectPassphrase
}

func (app *App) lockSecretStore() {
	app.secretStore = nil
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

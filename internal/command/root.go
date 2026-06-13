package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

var Version = "dev"

// App holds shared state for all commands.
type App struct {
	Store       *config.Store
	SecretPath  string
	secretStore *secret.FileStore
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
	case "--list", "-l", "list", "ls":
		return app.cmdList(args[1:])
	case "--add", "-a", "add":
		return app.cmdAdd(args[1:])
	case "--edit", "-e", "edit":
		return app.cmdEdit(args[1:])
	case "--delete", "-d", "--del", "delete", "del", "rm":
		return app.cmdDelete(args[1:])
	case "--copy", "--cp", "copy", "cp":
		return app.cmdCopy(args[1:])
	case "--show", "show", "info":
		return app.cmdShow(args[1:])
	case "--search", "-s", "--find", "search", "find":
		return app.cmdSearch(args[1:])
	case "--group", "-g", "group":
		return app.cmdGroup(args[1:])
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
	case "doctor":
		return app.cmdDoctor(args[1:])
	case "connect", "conn":
		return app.cmdConnect(args[1:])
	case "--ping", "-p", "ping":
		return app.cmdPing(args[1:])
	case "--exec", "-x", "exec":
		return app.cmdExec(args[1:])
	case "--exec-group", "--xg", "exec-group":
		return app.cmdExecGroup(args[1:])
	case "--exec-all", "--xa", "exec-all":
		return app.cmdExecAll(args[1:])
	case "--export-ssh-config", "export-ssh-config":
		return app.cmdExportSSHConfig(args[1:])
	case "--import-ssh-config", "import-ssh-config":
		return app.cmdImportSSHConfig(args[1:])
	case "--passwd", "passwd":
		return app.cmdPasswd(args[1:])
	case "--forget-pass", "forget-pass":
		return app.cmdForgetPass(args[1:])
	case "--import-key", "import-key":
		return app.cmdImportKey(args[1:])
	case "--gen-key", "gen-key":
		return app.cmdGenKey(args[1:])
	case "--show-pubkey", "show-pubkey":
		return app.cmdShowPubkey(args[1:])
	case "--auth", "auth":
		return app.cmdAuth(args[1:])
	case "--lock", "lock":
		app.lockSecretStore()
		ui.PrintSuccess("当前会话密码库已锁定")
		return nil
	case "--help", "-h", "help":
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
	commands := []string{
		"--list", "--add", "--edit", "--delete", "--copy", "--show", "--search",
		"--group", "--ping", "--exec", "--exec-group", "--exec-all", "--passwd",
		"--forget-pass", "--import-key", "--gen-key", "--show-pubkey", "--auth",
		"--lock", "--export-ssh-config", "--import-ssh-config", "--help", "--version",
	}
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
	fmt.Println("  sshm <别名|ID> [SSH参数...]    连接到主机")
	fmt.Println("  sshm <命令> [参数...]           执行管理命令")
	fmt.Println()
	fmt.Println("常用命令（旧版 --参数仍然兼容）:")
	fmt.Println("  list                          列出所有主机")
	fmt.Println("  add [别名 user@主机[:端口]]    添加主机")
	fmt.Println("  edit <别名|ID>                编辑主机")
	fmt.Println("  delete <别名|ID>              删除主机")
	fmt.Println("  copy <别名|ID>                复制连接命令")
	fmt.Println("  show <别名|ID>                显示主机详情")
	fmt.Println("  search <关键词...>            搜索主机")
	fmt.Println("  group [分组名]                列出分组")
	fmt.Println("  recent [数量]                 显示收藏和最近连接")
	fmt.Println("  pin/unpin <别名|ID>           收藏或取消收藏")
	fmt.Println("  completion <bash|zsh|fish>     生成 Shell 自动补全脚本")
	fmt.Println("  pick                          打开可搜索主机选择器")
	fmt.Println("  connect <别名|ID> [SSH参数...]  显式连接主机")
	fmt.Println("  doctor                        检查配置、SSH 与密钥环境")
	fmt.Println("  ping [别名|ID]                测试连接")
	fmt.Println("  exec <目标> <命令>             执行命令")
	fmt.Println("  exec-group <分组> <命令>       分组执行命令")
	fmt.Println("  exec-all <命令>                所有主机执行命令")
	fmt.Println("  passwd <别名|ID>              设置 SSH 密码")
	fmt.Println("  forget-pass <别名|ID>         删除 SSH 密码")
	fmt.Println("  import-key <别名|ID> <路径>    导入密钥")
	fmt.Println("  gen-key <别名|ID>             生成密钥")
	fmt.Println("  show-pubkey <别名|ID>         显示公钥")
	fmt.Println("  auth <别名|ID>                修改认证策略")
	fmt.Println("  lock                           锁定当前会话密码库")
	fmt.Println("  export-ssh-config [文件]      导出 SSH 配置")
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
	fmt.Printf("    %-14s %-24s %s\n", "list, ls, l", "列出所有主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "add, a", "添加主机", "")
	fmt.Printf("    %-14s %-24s %s\n", "edit, e", "编辑主机", "edit <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "del, delete, rm, d", "删除主机", "del <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "show, info", "显示主机详情", "show <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "search, find, s", "搜索主机", "search <关键词>")
	fmt.Printf("    %-14s %-24s %s\n", "group, g", "分组管理", "group [分组名]")
	fmt.Printf("    %-14s %-24s %s\n", "copy, cp", "复制连接信息", "copy <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "recent, r", "收藏与最近连接", "")
	fmt.Printf("    %-14s %-24s %s\n", "pin, unpin", "收藏或取消收藏", "pin <别名|ID>")
	fmt.Println()
	fmt.Println("  连接与执行")
	fmt.Printf("    %-14s %-24s %s\n", "conn, connect, c", "连接到主机", "c <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "pick", "打开可搜索主机选择器", "")
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
	fmt.Printf("    %-14s %-24s %s\n", "lock", "锁定当前会话密码库", "")
	fmt.Println()
	fmt.Println("  配置")
	fmt.Printf("    %-14s %-24s %s\n", "ssh-config, sc", "导入/导出 SSH 配置", "")
	fmt.Printf("    %-14s %-24s %s\n", "doctor", "检查本机环境", "")
	fmt.Println()
	fmt.Println("  其他")
	fmt.Printf("    %-14s %-24s %s\n", "help, h", "显示此帮助", "")
	fmt.Printf("    %-14s %-24s %s\n", "exit, quit, q", "退出程序", "")
	fmt.Println()
	fmt.Printf("  也可以直接输入 %s 快速连接主机。\n", ui.CyanText("别名 或 ID"))
	fmt.Println()
}

// requireSecretStore creates a FileStore, prompting for master password.
// If the secrets file already exists, it verifies the passphrase before proceeding.
func (app *App) requireSecretStore() (*secret.FileStore, error) {
	if app.secretStore != nil {
		return app.secretStore, nil
	}

	if _, err := os.Stat(app.SecretPath); os.IsNotExist(err) {
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
		app.secretStore = secret.NewFileStore(app.SecretPath, pass1)
		return app.secretStore, nil
	} else if err != nil {
		return nil, fmt.Errorf("检查 secrets 文件失败: %w", err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		pass, err := ui.ReadPassword("请输入 sshm 主密码: ")
		if err != nil {
			return nil, fmt.Errorf("读取主密码失败: %w", err)
		}
		fs := secret.NewFileStore(app.SecretPath, pass)
		if err := fs.VerifyPassphrase(); err == nil {
			app.secretStore = fs
			if err := app.migratePasswordRefs(fs); err != nil {
				app.secretStore = nil
				return nil, fmt.Errorf("迁移旧密码引用失败: %w", err)
			}
			return app.secretStore, nil
		} else if !errors.Is(err, secret.ErrIncorrectPassphrase) {
			return nil, fmt.Errorf("无法读取 secrets 文件: %w", err)
		} else if attempt < 3 {
			fmt.Fprintln(os.Stderr, ui.Warn("主密码错误，请重试 (%d/3)", attempt))
		}
	}
	return nil, secret.ErrIncorrectPassphrase
}

func (app *App) lockSecretStore() {
	app.secretStore = nil
}

func (app *App) migratePasswordRefs(fs *secret.FileStore) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	destToSource := map[string]string{}
	stableIDs := map[string]bool{}
	for _, h := range hf.Hosts {
		stableIDs[h.ID] = true
		if h.PasswordRef != "" && h.PasswordRef != h.ID {
			destToSource[h.ID] = h.PasswordRef
		}
	}
	if len(destToSource) == 0 {
		return nil
	}
	available := map[string]string{}
	for dest, source := range destToSource {
		if _, err := fs.GetPassword(source); err == nil {
			available[dest] = source
		}
	}
	if len(available) == 0 {
		return nil
	}
	if err := fs.CopyPasswords(available); err != nil {
		return err
	}

	oldRefs := make([]string, 0, len(destToSource))
	for i := range hf.Hosts {
		if source, ok := available[hf.Hosts[i].ID]; ok {
			hf.Hosts[i].PasswordRef = hf.Hosts[i].ID
			if !stableIDs[source] {
				oldRefs = append(oldRefs, source)
			}
		}
	}
	if err := app.Store.Save(hf); err != nil {
		return err
	}
	if err := fs.RemovePasswords(oldRefs...); err != nil {
		return fmt.Errorf("配置已迁移，但清理旧密码引用失败: %w", err)
	}
	return nil
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

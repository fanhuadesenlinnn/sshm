package command

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

var Version = "dev"

func CurrentVersion() string {
	moduleVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion = info.Main.Version
	}
	return resolveVersion(Version, moduleVersion)
}

func resolveVersion(injected, module string) string {
	if injected != "" && injected != "dev" {
		return injected
	}
	if module != "" && module != "(devel)" {
		return module
	}
	if injected != "" {
		return injected
	}
	return "dev"
}

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

// Run executes the Cobra command tree.
func Run(args []string) error {
	return runCobra(NewApp(), args)
}

func unknownOptionError(option string) error {
	best := ""
	bestDistance := 4
	for _, command := range legacyOptionCandidates() {
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
	for _, group := range commandGroups {
		fmt.Printf("  %s\n", group.Title)
		for _, entry := range commandCatalog {
			if entry.Group != group.ID || entry.HelpUsage == "" {
				continue
			}
			fmt.Printf("    %-32s %s\n", entry.HelpUsage, entry.HelpSummary)
		}
	}
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
	fmt.Printf("    %-14s %-24s %s\n", "tag, tags", "进入标签管理中心", "tag <子命令>")
	fmt.Printf("    %-14s %-24s %s\n", "recent, r", "收藏与最近连接", "")
	fmt.Printf("    %-14s %-24s %s\n", "pin, unpin", "收藏或取消收藏", "pin <别名|ID>")
	fmt.Println()
	fmt.Println("  连接与执行")
	fmt.Printf("    %-14s %-24s %s\n", "conn, connect, c", "连接到主机", "c <别名|ID>")
	fmt.Printf("    %-14s %-24s %s\n", "pick, p", "打开可搜索主机选择器", "")
	fmt.Printf("    %-14s %-24s %s\n", "ping", "测试连通性", "ping [别名|ID]")
	fmt.Printf("    %-14s %-24s %s\n", "exec, x", "远程执行命令", "x <别名|ID> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "exec-tag, xt", "按标签执行命令", "xt <标签> <命令>")
	fmt.Printf("    %-14s %-24s %s\n", "deploy", "批量部署工作流", "deploy <子命令>")
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
	fmt.Printf("    %-14s %-24s %s\n", "config edit", "校验后编辑 sshm.yaml", "")
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

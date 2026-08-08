package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
	"github.com/spf13/cobra"
)

const allowWithoutConfig = "allow-without-config"

func runCobra(app *App, args []string) error {
	root := newRootCommand(app)
	if handled, err := dispatchHelp(root, args); handled {
		return err
	}
	if handled, err := dispatchLegacyRootOption(app, args); handled {
		return err
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && !knownRootCommand(root, args[0]) {
		if _, err := os.Stat(app.ConfigPath); errors.Is(err, os.ErrNotExist) {
			return config.ErrNotInitialized
		} else if err != nil {
			return fmt.Errorf("检查配置文件失败: %w", err)
		}
		if _, _, _, err := app.Store.FindHost(args[0]); err != nil {
			return unknownCommandOrHostError(root, args[0], err)
		}
		return app.cmdConnect(args)
	}
	root.SetArgs(args)
	return root.Execute()
}

func newRootCommand(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "sshmd",
		Short:         "本地优先的个人 SSH 运维工具",
		Long:          "sshmd 是本地优先的个人 SSH 运维工具，用来管理主机、凭据、标签、批量命令、安全传输和轻量 Deploy 编排。",
		Example:       rootExamples(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       CurrentVersion(),
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.interactiveMode()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Parent() == nil {
				return nil
			}
			if cmd.Annotations[allowWithoutConfig] == "true" {
				return nil
			}
			if _, err := os.Stat(app.ConfigPath); errors.Is(err, os.ErrNotExist) {
				return config.ErrNotInitialized
			} else if err != nil {
				return fmt.Errorf("检查配置文件失败: %w", err)
			}
			return nil
		},
	}
	root.SetVersionTemplate("sshmd {{.Version}}\n")
	for _, group := range commandGroups {
		root.AddGroup(&cobra.Group{ID: group.ID, Title: group.Title})
	}

	root.AddCommand(newInitCommand(app))
	root.AddCommand(newConfigCommand(app))
	root.AddCommand(legacyCommand("doctor", "", app.cmdDoctor, allowWithoutConfig))
	root.AddCommand(legacyCommand("host", "", app.cmdHost))
	root.AddCommand(legacyCommand("key", "", app.cmdKey))
	root.AddCommand(legacyCommand("tag", "", app.cmdTag))
	root.AddCommand(newDeployCommand(app))
	root.AddCommand(legacyCommand("list", "", app.cmdList))
	root.AddCommand(legacyCommand("add", "", app.cmdAdd))
	root.AddCommand(legacyCommand("add-batch", "", app.cmdAddBatch))
	root.AddCommand(legacyCommand("edit", "", app.cmdEdit))
	root.AddCommand(legacyCommand("delete", "", app.cmdDelete))
	root.AddCommand(legacyCommand("show", "", app.cmdShow))
	root.AddCommand(legacyCommand("search", "", app.cmdSearch))
	root.AddCommand(legacyCommand("connect", "", app.cmdConnect))
	root.AddCommand(legacyCommand("ping", "", app.cmdPing))
	root.AddCommand(legacyCommand("exec", "", app.cmdExec))
	root.AddCommand(legacyCommand("exec-tag", "", app.cmdExecTag))
	root.AddCommand(legacyCommand("push", "", app.cmdPush))
	root.AddCommand(legacyCommand("pull", "", app.cmdPull))
	root.AddCommand(legacyCommand("push-tag", "", app.cmdPushTag))
	root.AddCommand(legacyCommand("pull-tag", "", app.cmdPullTag))
	root.AddCommand(legacyCommand("forward", "", app.cmdForward))
	root.AddCommand(legacyCommand("logs", "", app.cmdLogs))
	root.AddCommand(legacyCommand("recent", "", app.cmdRecent))
	root.AddCommand(legacyCommand("pin", "", func(args []string) error { return app.cmdPin(args, true) }))
	root.AddCommand(legacyCommand("unpin", "", func(args []string) error { return app.cmdPin(args, false) }))
	root.AddCommand(legacyCommand("find-con", "", app.cmdPick))
	root.AddCommand(legacyCommand("passwd", "", app.cmdPasswd))
	root.AddCommand(legacyCommand("forget-pass", "", app.cmdForgetPass))
	root.AddCommand(legacyCommand("show-pubkey", "", app.cmdShowPubkey))
	root.AddCommand(legacyCommand("auth", "", app.cmdAuth))
	root.AddCommand(legacyCommand("export-ssh-config", "", app.cmdExportSSHConfig))
	root.AddCommand(legacyCommand("import-ssh-config", "", app.cmdImportSSHConfig))
	root.AddCommand(legacyCommand("completion", "", app.cmdCompletion, allowWithoutConfig))
	root.AddCommand(&cobra.Command{
		Use:     "lock",
		Short:   commandShort("lock", "锁定当前会话密码库"),
		GroupID: commandGroupID("lock"),
		Args:    cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			app.lockSecretStore()
			ui.PrintSuccess("当前会话密码库已锁定")
		},
	})
	return root
}

func rootExamples() string {
	return strings.TrimSpace(`
sshmd init
sshmd add web01 root@10.0.0.11
sshmd passwd web01
sshmd ping web01
sshmd web01
sshmd exec-tag prod "uptime" --yes`)
}

func dispatchHelp(root *cobra.Command, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	last := args[len(args)-1]
	if !isHelpToken(last) {
		return false, nil
	}
	path := args[:len(args)-1]
	if len(path) == 0 {
		return true, root.Help()
	}
	command, remaining, err := root.Find(path)
	if err != nil || len(remaining) != 0 {
		return false, nil
	}
	if printCenterHelp(command.Name()) {
		return true, nil
	}
	return true, command.Help()
}

func printCenterHelp(name string) bool {
	switch name {
	case "host":
		printHostCenterHelp()
	case "key":
		printKeyCenterHelp()
	case "tag":
		printTagCenterHelp()
	case "logs":
		printLogsHelp()
	case "completion":
		printCompletionHelp()
	default:
		return false
	}
	return true
}

func legacyCommand(use, short string, run func([]string) error, annotations ...string) *cobra.Command {
	return legacyCommandWithAliases(use, short, run, nil, annotations...)
}

func legacyCommandWithAliases(use, short string, run func([]string) error, aliases []string, annotations ...string) *cobra.Command {
	name := strings.Fields(use)[0]
	if short == "" {
		short = commandShort(name, short)
	}
	if len(aliases) == 0 {
		aliases = commandAliases(name)
	}
	cmd := &cobra.Command{
		Use:                use,
		Aliases:            aliases,
		Short:              short,
		GroupID:            commandGroupID(name),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
				return cmd.Help()
			}
			return run(args)
		},
	}
	applyLegacyHelp(cmd)
	for _, annotation := range annotations {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[annotation] = "true"
	}
	return cmd
}

func isHelpToken(value string) bool {
	return value == "-h" || value == "--help" || value == "help"
}

func containsHelpToken(args []string) bool {
	for _, arg := range args {
		if isHelpToken(arg) {
			return true
		}
	}
	return false
}

type commandHelp struct {
	use     string
	long    string
	example string
}

var legacyHelp = map[string]commandHelp{
	"add": {
		use:  "add <别名> <用户@主机[:端口]> [选项]",
		long: "添加一台主机。默认认证策略为 auto，可以之后用 passwd 保存密码，或用 key setup 绑定托管密钥。",
		example: strings.TrimSpace(`
sshmd add web01 root@10.0.0.11
sshmd add web01 deploy@10.0.0.11:2222 --tags prod,web
sshmd passwd web01`),
	},
	"connect": {
		use:  "connect <别名|ID>",
		long: "连接到一台已保存主机。也可以直接运行 sshmd <别名|ID> 快速连接。",
		example: strings.TrimSpace(`
sshmd web01
sshmd connect web01`),
	},
	"exec": {
		use:  "exec [--yes] [--quiet] [--no-log] <别名|ID> [--] <命令>",
		long: "在单台主机上执行远程命令。复杂命令建议使用 -- 标记命令起点。默认会在交互终端确认并写入操作日志；脚本或 CI 中请显式传入 --yes。",
		example: strings.TrimSpace(`
sshmd exec web01 "uptime"
sshmd exec --yes web01 "systemctl status app"
sshmd exec --yes --quiet --no-log web01 "hostname"`),
	},
	"exec-tag": {
		use:  "exec-tag [批量选项] <标签|all> [--] <命令>",
		long: "按标签批量执行远程命令。复杂命令建议使用 -- 标记命令起点。使用虚拟标签 all 可选择全部主机；批量操作默认需要确认。",
		example: strings.TrimSpace(`
sshmd exec-tag prod "uptime" --yes
sshmd exec-tag all "hostname" --parallel 8 --connect-timeout 5s --yes
sshmd exec-tag web "systemctl restart app" --serial 2 --max-fail 1 --yes`),
	},
	"ping": {
		use:  "ping [--yes] [--quiet] [别名|ID]",
		long: "测试主机 SSH 连接。不给目标时会测试全部主机；多主机模式默认需要确认。",
		example: strings.TrimSpace(`
sshmd ping web01
sshmd ping --yes --quiet`),
	},
	"push": {
		use:  "push [选项] <别名|ID> <本地路径> <远程路径>",
		long: "向单台主机推送文件或目录。默认拒绝覆盖不同内容；需要覆盖时显式使用 --overwrite 或 --backup。",
		example: strings.TrimSpace(`
sshmd push web01 ./dist/app.tar.gz /opt/app/app.tar.gz --yes
sshmd push web01 ./dist /opt/app --backup --yes
sshmd push web01 ./dist /opt/app --method rsync --yes`),
	},
	"pull": {
		use:  "pull [选项] <别名|ID> <远程路径> <本地路径>",
		long: "从单台主机拉取文件或目录。默认拒绝覆盖不同内容；需要覆盖时显式使用 --overwrite 或 --backup。",
		example: strings.TrimSpace(`
sshmd pull web01 /etc/nginx/nginx.conf ./nginx.web01.conf --yes
sshmd pull web01 /var/log/app ./logs/web01 --backup --yes`),
	},
	"push-tag": {
		use:  "push-tag [批量选项] <标签|all> <本地路径> <远程路径>",
		long: "按标签批量推送文件或目录。批量操作默认需要确认，并会写入操作日志。",
		example: strings.TrimSpace(`
sshmd push-tag prod ./dist/app.tar.gz /opt/app/app.tar.gz --backup --yes
sshmd push-tag all ./script.sh /tmp/script.sh --parallel 8 --yes`),
	},
	"pull-tag": {
		use:  "pull-tag [批量选项] <标签|all> <远程路径> <本地路径>",
		long: "按标签批量拉取文件或目录。默认按主机别名分目录保存；需要平铺时使用 --flat。",
		example: strings.TrimSpace(`
sshmd pull-tag prod /etc/hosts ./backup --yes
sshmd pull-tag all /etc/nginx ./backup --flat --yes`),
	},
	"key": {
		use:  "key <命令> [参数]",
		long: "管理 sshmd 托管密钥。常用流程是 create/import 创建本地托管密钥，再用 setup 推送、验证并绑定主机。",
		example: strings.TrimSpace(`
sshmd key list
sshmd key create personal --default
sshmd key import personal ~/.ssh/id_ed25519 --default
sshmd key setup personal web01 --yes`),
	},
	"host": {
		use:  "host <命令> [参数]",
		long: "管理 SSH 主机。可以列出、添加、批量添加、编辑、删除主机，也可以导入 OpenSSH 配置。",
		example: strings.TrimSpace(`
sshmd host list
sshmd host add web01 root@10.0.0.11
sshmd host delete web01 --yes
sshmd host import-ssh-config ~/.ssh/config`),
	},
	"tag": {
		use:  "tag <命令> [参数]",
		long: "管理主机标签。标签用于搜索、分组和批量执行；虚拟标签 all 表示全部主机。",
		example: strings.TrimSpace(`
sshmd tag list
sshmd tag create prod --note 生产环境
sshmd tag add prod web01 web02
sshmd exec-tag prod "uptime" --yes`),
	},
	"passwd": {
		use:  "passwd <目标...|--tag 标签|--all> [--yes]",
		long: "为一台或多台主机保存同一个 SSH 密码。密码会写入 sshmd.yaml 内的加密 vault，需要主密码解锁。",
		example: strings.TrimSpace(`
sshmd passwd web01
sshmd passwd web01 web02
sshmd passwd --tag prod
sshmd ping web01`),
	},
	"forget-pass": {
		use:  "forget-pass <目标...|--tag 标签|--all> [--yes]",
		long: "删除一台或多台主机保存的 SSH 密码。删除前默认需要确认；脚本或 CI 中请显式传入 --yes。",
		example: strings.TrimSpace(`
sshmd forget-pass web01
sshmd forget-pass --tag prod
sshmd forget-pass --all --yes`),
	},
	"logs": {
		use:  "logs [clean --yes]",
		long: "查看或清理本地操作日志。日志可能包含敏感远程输出，清理前默认需要确认。",
		example: strings.TrimSpace(`
sshmd logs
sshmd logs clean --yes`),
	},
	"completion": {
		use:  "completion <bash|zsh|fish>",
		long: "生成 Shell 自动补全脚本。该命令不需要先运行 sshmd init。",
		example: strings.TrimSpace(`
sshmd completion zsh > ~/.zsh/completions/_sshm
sshmd completion bash > ~/.local/share/bash-completion/completions/sshmd`),
	},
	"forward": {
		use:  "forward <别名|ID> <本地监听地址> <远程目标>",
		long: "建立本地端口转发。适合临时访问远端内网服务。",
		example: strings.TrimSpace(`
sshmd forward prod 127.0.0.1:8080 127.0.0.1:80
sshmd forward prod 127.0.0.1:15432 127.0.0.1:5432`),
	},
}

func applyLegacyHelp(cmd *cobra.Command) {
	help, ok := legacyHelp[cmd.Name()]
	if !ok {
		return
	}
	cmd.Use = help.use
	cmd.Long = help.long
	cmd.Example = help.example
}

func knownRootCommand(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if command.Name() == name {
			return true
		}
		for _, alias := range command.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return name == "help"
}

// rootCommandNames returns all registered root command names and aliases,
// used as candidates for spell-check suggestions on unknown input.
func rootCommandNames(root *cobra.Command) []string {
	names := []string{"help"}
	for _, command := range root.Commands() {
		names = append(names, command.Name())
		names = append(names, command.Aliases...)
	}
	return names
}

// unknownCommandOrHostError builds a clearer error for input that is neither a
// known command nor a known host. It reuses the edit-distance spell-check so a
// near miss like "lst" suggests "list".
func unknownCommandOrHostError(root *cobra.Command, input string, cause error) error {
	best := ""
	bestDistance := 3
	for _, name := range rootCommandNames(root) {
		if distance := editDistance(input, name); distance < bestDistance {
			best = name
			bestDistance = distance
		}
	}
	if best != "" {
		return fmt.Errorf("未知命令或主机 %q；你是否想使用 %q？（使用 sshmd --help 查看可用命令）: %w", input, best, cause)
	}
	return fmt.Errorf("未知命令或主机 %q；使用 sshmd --help 查看可用命令: %w", input, cause)
}

func dispatchLegacyRootOption(app *App, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	handlers := map[string]func([]string) error{
		"--list": app.cmdList, "-l": app.cmdList,
		"--add": app.cmdAdd, "-a": app.cmdAdd,
		"--edit": app.cmdEdit, "-e": app.cmdEdit,
		"--delete": app.cmdDelete, "--del": app.cmdDelete, "-d": app.cmdDelete,
		"--show":   app.cmdShow,
		"--search": app.cmdSearch, "--find": app.cmdSearch, "-s": app.cmdSearch,
		"--tag":  app.cmdTag,
		"--ping": app.cmdPing, "-p": app.cmdPing,
		"--exec": app.cmdExec, "-x": app.cmdExec,
		"--exec-tag": app.cmdExecTag, "--xt": app.cmdExecTag,
		"--export-ssh-config": app.cmdExportSSHConfig,
		"--import-ssh-config": app.cmdImportSSHConfig,
		"--passwd":            app.cmdPasswd,
		"--forget-pass":       app.cmdForgetPass,
		"--show-pubkey":       app.cmdShowPubkey,
		"--auth":              app.cmdAuth,
		"--lock": func([]string) error {
			app.lockSecretStore()
			ui.PrintSuccess("当前会话密码库已锁定")
			return nil
		},
	}
	handler, ok := handlers[args[0]]
	if !ok {
		if strings.HasPrefix(args[0], "-") && args[0] != "-h" && args[0] != "--help" && args[0] != "-v" && args[0] != "--version" {
			return true, unknownOptionError(args[0])
		}
		return false, nil
	}
	if _, err := os.Stat(app.ConfigPath); errors.Is(err, os.ErrNotExist) {
		return true, config.ErrNotInitialized
	} else if err != nil {
		return true, fmt.Errorf("检查配置文件失败: %w", err)
	}
	return true, handler(args[1:])
}

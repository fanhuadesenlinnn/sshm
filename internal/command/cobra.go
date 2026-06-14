package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
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
			return fmt.Errorf("未知命令或主机 %q；使用 sshm --help 查看可用命令: %w", args[0], err)
		}
		return app.cmdConnect(args)
	}
	root.SetArgs(args)
	return root.Execute()
}

func newRootCommand(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "sshm",
		Short:         "本地优先的个人 SSH 运维工具",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       CurrentVersion(),
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.interactiveMode()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
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
	root.SetVersionTemplate("sshm {{.Version}}\n")

	root.AddCommand(newInitCommand(app))
	root.AddCommand(newConfigCommand(app))
	root.AddCommand(legacyCommand("doctor", "检查配置与凭据环境", app.cmdDoctor, allowWithoutConfig))
	root.AddCommand(legacyCommand("host", "管理主机", app.cmdHost))
	root.AddCommand(legacyCommandWithAliases("key", "管理托管密钥", app.cmdKey, []string{"k"}))
	root.AddCommand(legacyCommandWithAliases("tag", "管理标签", app.cmdTag, []string{"tags"}))
	root.AddCommand(newDeployCommand(app))
	root.AddCommand(legacyCommandWithAliases("list", "列出主机", app.cmdList, []string{"ls"}))
	root.AddCommand(legacyCommand("add", "添加主机", app.cmdAdd))
	root.AddCommand(legacyCommand("add-batch", "批量添加主机", app.cmdAddBatch))
	root.AddCommand(legacyCommand("edit", "编辑主机", app.cmdEdit))
	root.AddCommand(legacyCommandWithAliases("delete", "删除主机", app.cmdDelete, []string{"del", "rm"}))
	root.AddCommand(legacyCommandWithAliases("show", "显示主机详情", app.cmdShow, []string{"info"}))
	root.AddCommand(legacyCommandWithAliases("search", "搜索主机", app.cmdSearch, []string{"find"}))
	root.AddCommand(legacyCommandWithAliases("connect", "连接主机", app.cmdConnect, []string{"conn"}))
	root.AddCommand(legacyCommand("ping", "测试 SSH 连接", app.cmdPing))
	root.AddCommand(legacyCommand("exec", "在单台主机执行命令", app.cmdExec))
	root.AddCommand(legacyCommand("exec-tag", "按标签批量执行命令", app.cmdExecTag))
	root.AddCommand(legacyCommand("push", "向单台主机推送文件或目录", app.cmdPush))
	root.AddCommand(legacyCommand("pull", "从单台主机拉取文件或目录", app.cmdPull))
	root.AddCommand(legacyCommand("push-tag", "按标签批量推送文件或目录", app.cmdPushTag))
	root.AddCommand(legacyCommand("pull-tag", "按标签批量拉取文件或目录", app.cmdPullTag))
	root.AddCommand(legacyCommand("forward", "建立本地端口转发", app.cmdForward))
	root.AddCommand(legacyCommand("logs", "查看或清理操作日志", app.cmdLogs))
	root.AddCommand(legacyCommand("recent", "显示收藏和最近连接", app.cmdRecent))
	root.AddCommand(legacyCommand("pin", "收藏主机", func(args []string) error { return app.cmdPin(args, true) }))
	root.AddCommand(legacyCommand("unpin", "取消收藏主机", func(args []string) error { return app.cmdPin(args, false) }))
	root.AddCommand(legacyCommand("pick", "打开主机选择器", app.cmdPick))
	root.AddCommand(legacyCommand("passwd", "设置 SSH 密码", app.cmdPasswd))
	root.AddCommand(legacyCommand("forget-pass", "删除 SSH 密码", app.cmdForgetPass))
	root.AddCommand(legacyCommand("show-pubkey", "显示托管公钥", app.cmdShowPubkey))
	root.AddCommand(legacyCommand("auth", "修改认证策略", app.cmdAuth))
	root.AddCommand(legacyCommand("export-ssh-config", "导出 OpenSSH 配置", app.cmdExportSSHConfig))
	root.AddCommand(legacyCommand("import-ssh-config", "导入 OpenSSH 配置", app.cmdImportSSHConfig))
	root.AddCommand(legacyCommand("completion", "生成 Shell 自动补全脚本", app.cmdCompletion))
	root.AddCommand(&cobra.Command{
		Use:   "lock",
		Short: "锁定当前会话密码库",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			app.lockSecretStore()
			ui.PrintSuccess("当前会话密码库已锁定")
		},
	})
	return root
}

func dispatchHelp(root *cobra.Command, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	last := args[len(args)-1]
	if last != "-h" && last != "--help" && last != "help" {
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
	return true, command.Help()
}

func legacyCommand(use, short string, run func([]string) error, annotations ...string) *cobra.Command {
	return legacyCommandWithAliases(use, short, run, nil, annotations...)
}

func legacyCommandWithAliases(use, short string, run func([]string) error, aliases []string, annotations ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                use,
		Aliases:            aliases,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
				return cmd.Help()
			}
			return run(args)
		},
	}
	for _, annotation := range annotations {
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[annotation] = "true"
	}
	return cmd
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
		"--ping": app.cmdPing, "-p": app.cmdPing,
		"--exec": app.cmdExec, "-x": app.cmdExec,
		"--exec-tag": app.cmdExecTag, "--xt": app.cmdExecTag,
		"--export-ssh-config": app.cmdExportSSHConfig,
		"--import-ssh-config": app.cmdImportSSHConfig,
		"--passwd":            app.cmdPasswd,
		"--forget-pass":       app.cmdForgetPass,
		"--show-pubkey":       app.cmdShowPubkey,
		"--auth":              app.cmdAuth,
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

package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
	"github.com/spf13/cobra"
)

func newInitCommand(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:         "init",
		Short:       "初始化 ~/.sshm 工作目录和 v2 配置",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{allowWithoutConfig: "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.cmdInit(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "备份并覆盖已有 sshm.yaml")
	return cmd
}

func newConfigCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "查看和编辑主配置",
		RunE: func(_ *cobra.Command, args []string) error {
			return app.cmdConfig(args)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:         "path",
		Short:       "显示当前 sshm 路径",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{allowWithoutConfig: "true"},
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.cmdConfigPath()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "使用编辑器校验后更新 sshm.yaml",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.cmdConfigEdit(nil)
		},
	})
	return cmd
}

func (app *App) cmdInit(force bool) error {
	paths, backup, err := config.Initialize(force)
	if err != nil {
		return err
	}
	if legacy, exists := config.LegacyConfigExists(); exists {
		ui.PrintWarn("发现旧配置 %s；v6 不会读取、迁移或删除该文件", legacy)
	}
	fmt.Println("initialized sshm home:")
	printPaths(paths)
	if backup != "" {
		fmt.Printf("backup:   %s\n", backup)
	}
	return nil
}

func (app *App) cmdConfigPath() error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	printPaths(paths)
	return nil
}

func printPaths(paths config.Paths) {
	fmt.Printf("home:     %s\n", paths.Home)
	fmt.Printf("config:   %s\n", paths.Config)
	fmt.Printf("logs:     %s\n", paths.Logs)
	fmt.Printf("deploy:   %s\n", paths.Deploy)
	fmt.Printf("deploy.d: %s\n", paths.DeployDir)
	fmt.Printf("backups:  %s\n", paths.Backups)
	fmt.Printf("tmp:      %s\n", paths.Temp)
}

func (app *App) cmdConfig(args []string) error {
	if len(args) > 0 && args[0] == "help" {
		printConfigHelp()
		return nil
	}
	if len(args) == 0 || args[0] == "show" {
		doc, err := app.Store.Repository().Load()
		if err != nil {
			return err
		}
		fmt.Printf("配置文件: %s\n", app.Store.Path())
		fmt.Printf("默认主机信任策略: %s\n", doc.Defaults.HostKeyPolicy)
		return nil
	}
	if len(args) == 2 && args[0] == "host-key-policy" {
		if !config.ValidHostKeyPolicy(args[1]) {
			return fmt.Errorf("策略必须是 strict、accept-new 或 insecure")
		}
		if err := app.Store.Repository().Update(func(doc *config.Document) error {
			doc.Defaults.HostKeyPolicy = args[1]
			return nil
		}); err != nil {
			return err
		}
		ui.PrintSuccess("默认主机信任策略已设置为 %s", args[1])
		return nil
	}
	return fmt.Errorf("用法: sshm config [show | host-key-policy <strict|accept-new|insecure>]")
}

func printConfigHelp() {
	fmt.Println("sshm 核心状态只使用一个文件：<SSHM_HOME>/sshm.yaml")
	fmt.Println("deploy 工作流使用独立 deploy.yaml、deploy.d/*.yaml 或显式 --file")
	fmt.Println("主机信任策略：strict | accept-new | insecure")
	fmt.Println("主机的 host_key_policy 为空时继承 defaults.host_key_policy")
	fmt.Println("标签定义保存在 tags.items；主机引用的新标签会自动登记")
	fmt.Println("host_trust 与 vault 由 sshm 管理，请勿手动修改")
	fmt.Println("使用 sshm config edit 校验后编辑完整配置")
	fmt.Println("v6 不读取或迁移旧 ~/.config/sshm/sshm.yaml")
}

package command

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

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
	fmt.Println("deploy 工作流使用独立 deploy.yaml、deploy.d/*.yaml 或项目 sshm.deploy.yaml")
	fmt.Println("主机信任策略：strict | accept-new | insecure")
	fmt.Println("主机的 host_key_policy 为空时继承 defaults.host_key_policy")
	fmt.Println("标签定义保存在 tags.items；主机引用的新标签会自动登记")
	fmt.Println("host_trust 与 vault 由 sshm 管理，请勿手动修改")
	fmt.Println("使用 sshm config-edit 校验后编辑完整配置")
	fmt.Println("v5 不读取或迁移旧版 hosts.yaml、keys.yaml、secrets.yaml 与 .bak")
}

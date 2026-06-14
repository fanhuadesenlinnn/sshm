package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdHost(args []string) error {
	if len(args) == 0 {
		if ui.IsTerminal() {
			return app.hostCenter()
		}
		app.printHostHelp()
		return nil
	}
	switch args[0] {
	case "list", "ls", "l":
		return app.cmdList(args[1:])
	case "add", "a":
		return app.cmdAdd(args[1:])
	case "add-batch", "batch", "ab":
		return app.cmdAddBatch(args[1:])
	case "edit", "e":
		return app.cmdEdit(args[1:])
	case "delete", "del", "rm", "d":
		return app.cmdDelete(args[1:])
	case "config-edit", "config":
		return app.cmdConfigEdit(args[1:])
	case "import-ssh-config", "import":
		return app.cmdImportSSHConfig(args[1:])
	case "help", "h":
		app.printHostHelp()
		return nil
	default:
		return fmt.Errorf("未知 host 命令 %q；使用 sshm host help 查看帮助", args[0])
	}
}

func (app *App) hostCenter() error {
	app.printHostHelp()
	for {
		parts := parseArgs(ui.ReadLine(ui.CyanText("host> ")))
		if len(parts) == 0 {
			continue
		}
		if parts[0] == "q" || parts[0] == "quit" || parts[0] == "back" {
			return nil
		}
		if err := app.cmdHost(parts); err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

func (app *App) printHostHelp() {
	ui.PrintHeader("主机管理中心")
	fmt.Println()
	fmt.Println("  l/list                         列出主机")
	fmt.Println("  a/add [别名 用户@主机[:端口]]  添加单台主机")
	fmt.Println("  ab/add-batch [别名=目标...]     批量添加主机")
	fmt.Println("  e/edit <别名|ID>               交互式编辑主机")
	fmt.Println("  d/delete <别名|ID>             删除主机")
	fmt.Println("  config-edit                    使用 $EDITOR 校验后更新 sshm.yaml")
	fmt.Println("  import-ssh-config [路径]       导入 OpenSSH 配置")
	fmt.Println()
	fmt.Println("  输入 back/q 返回主命令页")
	fmt.Println()
}

func (app *App) cmdAddBatch(args []string) error {
	if len(args) == 0 {
		fmt.Println("逐行输入 别名=用户@主机[:端口]，空行结束：")
		for {
			line := ui.ReadLine("host: ")
			if line == "" {
				break
			}
			args = append(args, line)
		}
	}
	if len(args) == 0 {
		return fmt.Errorf("没有输入主机")
	}
	added := make([]config.Host, 0, len(args))
	failed := 0
	for _, spec := range args {
		alias, target, ok := strings.Cut(spec, "=")
		if !ok {
			fields := strings.Fields(spec)
			if len(fields) == 2 {
				alias, target, ok = fields[0], fields[1], true
			}
		}
		if !ok || alias == "" || target == "" {
			failed++
			ui.PrintError("%s: 应使用 别名=用户@主机[:端口] 格式", spec)
			continue
		}
		h := config.DefaultHost()
		h.Alias = alias
		var err error
		h.User, h.Host, h.Port, err = parseSSHTarget(target)
		if err == nil {
			err = validateHost(h)
		}
		if err == nil {
			err = app.Store.Add(h)
		}
		if err != nil {
			failed++
			ui.PrintError("%s: 添加失败: %v", alias, err)
			continue
		}
		added = append(added, h)
		ui.PrintSuccess("已添加：%s", alias)
	}
	fmt.Printf("批量添加完成：成功 %d，失败 %d\n", len(added), failed)
	if failed > 0 {
		return fmt.Errorf("有 %d 台主机添加失败", failed)
	}
	return nil
}

func (app *App) cmdConfigEdit(_ []string) error {
	if _, err := app.Store.Repository().Load(); err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	data, err := os.ReadFile(app.Store.Path())
	if err != nil {
		return fmt.Errorf("读取当前配置失败: %w", err)
	}
	dir := filepath.Dir(app.Store.Path())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".sshm-edit-*.yaml")
	if err != nil {
		return fmt.Errorf("创建临时编辑文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("编辑器退出异常，配置未修改: %w", err)
	}
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("读取编辑后配置失败: %w", err)
	}
	validated, err := config.ValidateDocumentData(edited)
	if err != nil {
		return fmt.Errorf("配置校验失败，原配置未修改: %w", err)
	}
	if err := app.Store.Repository().Replace(validated); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	ui.PrintSuccess("配置已校验并更新：%s", app.Store.Path())
	return nil
}

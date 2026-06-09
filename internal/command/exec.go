package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/sshm/sshm/internal/secret"
	"github.com/sshm/sshm/internal/sshx"
	"github.com/sshm/sshm/internal/ui"
)

func (app *App) cmdExec(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm --exec <别名|ID> <命令>")
	}

	h, _, _, err := app.Store.FindHost(args[0])
	if err != nil {
		return err
	}

	command := strings.Join(args[1:], " ")

	fs := app.tryGetSecretStore()
	output, err := sshx.ExecCommand(*h, fs, command)
	if err != nil {
		ui.PrintError("执行失败: %v", err)
	}
	fmt.Print(output)
	return err
}

func (app *App) cmdExecGroup(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm --exec-group <分组> <命令>")
	}

	group := args[0]
	command := strings.Join(args[1:], " ")

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	fs := app.tryGetSecretStore()

	for _, h := range hf.Hosts {
		if h.Group != group {
			continue
		}
		fmt.Println()
		ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", h.Alias, h.User, h.Host))
		output, err := sshx.ExecCommand(h, fs, command)
		if err != nil {
			ui.PrintError("执行失败: %v", err)
		}
		fmt.Print(output)
	}
	return nil
}

func (app *App) cmdExecAll(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("用法: sshm --exec-all <命令>")
	}

	command := strings.Join(args, " ")

	hf, err := app.Store.Load()
	if err != nil {
		return err
	}

	fs := app.tryGetSecretStore()

	for _, h := range hf.Hosts {
		fmt.Println()
		ui.PrintHeader(fmt.Sprintf("=== %s (%s@%s) ===", h.Alias, h.User, h.Host))
		output, err := sshx.ExecCommand(h, fs, command)
		if err != nil {
			ui.PrintError("执行失败: %v", err)
		}
		fmt.Print(output)
	}
	return nil
}

// tryGetSecretStore attempts to create a secret store, prompting for master password.
// Returns nil if no secrets file exists, stdin is not a terminal, or authentication fails.
func (app *App) tryGetSecretStore() *secret.FileStore {
	// If no secrets file exists, return nil
	if _, err := os.Stat(app.SecretPath); os.IsNotExist(err) {
		return nil
	}
	// Only prompt if stdin is a terminal
	if !ui.IsTerminal() {
		return nil
	}
	pass, err := ui.ReadPassword("请输入 sshm 主密码: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.Warn("读取主密码失败，将使用系统 SSH"))
		return nil
	}
	fs := secret.NewFileStore(app.SecretPath, pass)
	if err := fs.VerifyPassphrase(); err != nil {
		fmt.Fprintln(os.Stderr, ui.Warn("主密码错误，将使用系统 SSH"))
		return nil
	}
	return fs
}

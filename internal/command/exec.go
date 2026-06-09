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

// tryGetSecretStore attempts to create a secret store without failing.
func (app *App) tryGetSecretStore() *secret.FileStore {
	// If no secrets file exists, return nil
	if _, err := os.Stat(app.SecretPath); os.IsNotExist(err) {
		return nil
	}
	return nil // Skip password prompt for batch exec; use system ssh
}

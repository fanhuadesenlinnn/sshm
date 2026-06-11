package command

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/internal/ui"
)

func (app *App) cmdCopy(args []string) error {
	h, _, _, err := app.resolveHost(args, "请输入要复制的主机别名或ID: ")
	if err != nil {
		return err
	}

	connStr := fmt.Sprintf("ssh -p %d %s@%s", h.Port, h.User, h.Host)
	if err := copyToClipboard(connStr); err != nil {
		ui.PrintWarn("无法写入系统剪贴板，将连接命令显示在下方: %v", err)
	} else {
		ui.PrintSuccess("连接命令已复制到剪贴板")
	}
	fmt.Println()
	ui.PrintHeader("SSH 连接命令")
	fmt.Println()
	fmt.Printf("  %s\n", connStr)
	fmt.Println()
	fmt.Println("  提示：可直接使用 sshm " + h.Alias + " 连接")
	fmt.Println()
	return nil
}

func copyToClipboard(text string) error {
	var commands [][]string
	switch runtime.GOOS {
	case "darwin":
		commands = [][]string{{"pbcopy"}}
	case "windows":
		commands = [][]string{{"cmd", "/c", "clip"}}
	default:
		commands = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	var lastErr error
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

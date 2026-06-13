package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
	"golang.org/x/crypto/ssh"
)

func (app *App) cmdPush(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm push <本地文件> <远程路径> <目标...>\n" +
			"  目标支持: 别名... --tag 标签 --all")
	}
	return app.cmdTransfer(args, "push")
}

func (app *App) cmdPull(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm pull <远程路径> <本地目录> <目标...>\n" +
			"  目标支持: 别名... --tag 标签 --all\n" +
			"  拉取的文件将按主机名分目录存放")
	}
	return app.cmdTransfer(args, "pull")
}

func (app *App) cmdTransfer(args []string, direction string) error {
	localPath := args[0]
	remotePath := args[1]
	targetArgs := args[2:]

	hosts, err := app.selectHosts(targetArgs)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return fmt.Errorf("目标选择结果为空")
	}

	// List and confirm.
	fmt.Println()
	verb := "推送"
	if direction == "pull" {
		verb = "拉取"
	}
	fmt.Printf("即将%s到 %d 台主机:\n", verb, len(hosts))
	for _, h := range hosts {
		fmt.Printf("  - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
	}
	fmt.Println()
	if !ui.ReadYesNo(fmt.Sprintf("确认%s? [y/N]: ", verb)) {
		ui.PrintWarn("已取消")
		return nil
	}

	// Try rsync first for push, then fall back to SSH cat pipe.
	fs := app.tryGetSecretStore()

	results := make([]transferResult, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(4, len(hosts))
	progress := &sync.Mutex{}
	done := 0

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				err := transferOne(ctx, hosts[i], fs, direction, localPath, remotePath)
				cancel()
				results[i] = transferResult{host: hosts[i], err: err}

				progress.Lock()
				done++
				status := ui.Success("ok")
				if err != nil {
					status = ui.ErrorMsg("FAIL")
				}
				fmt.Printf("\r  [%d/%d] %-18s %s", done, len(hosts), hosts[i].Alias, status)
				progress.Unlock()
			}
		}()
	}

	for i := range hosts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	fmt.Println()
	fmt.Println()

	// Summary.
	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
		}
	}
	fmt.Printf("%s完成: 成功 %d, 失败 %d\n", verb, len(results)-failed, failed)
	if failed > 0 {
		fmt.Println("\n失败详情:")
		for _, r := range results {
			if r.err != nil {
				ui.PrintError("  %s: %v", r.host.Alias, r.err)
			}
		}
		return fmt.Errorf("有 %d 台主机传输失败", failed)
	}
	return nil
}

type transferResult struct {
	host config.Host
	err  error
}

// transferOne handles a single file transfer to one host.
func transferOne(ctx context.Context, h config.Host, fs interface {
	GetPassword(string) (string, error)
	GetManagedKey(string) ([]byte, error)
}, direction, localPath, remotePath string) error {
	// Try rsync first.
	if direction == "push" {
		if err := transferRsync(ctx, h, fs, direction, localPath, remotePath); err == nil {
			return nil
		}
	}
	// Fallback to SSH pipe transfer.
	return transferViaSSH(ctx, h, fs, direction, localPath, remotePath)
}

// transferRsync uses system rsync for fast file transfer.
func transferRsync(ctx context.Context, h config.Host, fs interface {
	GetPassword(string) (string, error)
	GetManagedKey(string) ([]byte, error)
}, direction, localPath, remotePath string) error {
	rsyncPath, err := exec.LookPath("rsync")
	if err != nil {
		return fmt.Errorf("rsync不可用")
	}

	sshCmd, cleanup, err := buildRsyncSSH(h, fs)
	if err != nil {
		return err
	}
	defer cleanup()

	remote := fmt.Sprintf("%s@%s:%s", h.User, h.Host, remotePath)
	if h.Port != 22 {
		remote = fmt.Sprintf("%s@%s:%s", h.User, h.Host, remotePath)
	}

	var args []string
	if direction == "push" {
		args = []string{"-avz", "-e", sshCmd, localPath, remote}
	} else {
		args = []string{"-avz", "-e", sshCmd, remote, localPath}
	}

	cmd := exec.CommandContext(ctx, rsyncPath, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildRsyncSSH builds an ssh command string for rsync's -e flag.
// Writes the managed key to a temp file if needed.
func buildRsyncSSH(h config.Host, fs interface {
	GetManagedKey(string) ([]byte, error)
}) (string, func(), error) {
	sshBin := "ssh"
	if p, err := exec.LookPath("ssh"); err == nil {
		sshBin = p
	}

	noop := func() {}

	parts := []string{sshBin, "-o", "StrictHostKeyChecking=accept-new"}
	if h.Port != 22 {
		parts = append(parts, "-p", fmt.Sprintf("%d", h.Port))
	}

	// If managed key, write to temp file.
	if _, managed := config.ManagedKeyName(h.Identity); managed {
		if fs == nil {
			return "", noop, fmt.Errorf("托管密钥需要先解锁密码库")
		}
		name, _ := config.ManagedKeyName(h.Identity)
		keyData, err := fs.GetManagedKey(name)
		if err != nil {
			return "", noop, err
		}
		tmpFile, err := os.CreateTemp("", "sshm-rsync-key-*")
		if err != nil {
			return "", noop, fmt.Errorf("创建临时密钥文件失败: %w", err)
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.Write(keyData); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", noop, err
		}
		tmpFile.Chmod(0600)
		tmpFile.Close()
		cleanup := func() { os.Remove(tmpPath) }
		parts = append(parts, "-i", tmpPath)
		return strings.Join(parts, " "), cleanup, nil
	}

	return strings.Join(parts, " "), noop, nil
}

// transferViaSSH uses a Go SSH session with pipes for file transfer.
func transferViaSSH(ctx context.Context, h config.Host, fs interface {
	GetPassword(string) (string, error)
	GetManagedKey(string) ([]byte, error)
}, direction, localPath, remotePath string) error {
	// Get auth method.
	var authMethods []ssh.AuthMethod

	if name, managed := config.ManagedKeyName(h.Identity); managed {
		if fs == nil {
			return fmt.Errorf("托管密钥需要先解锁密码库")
		}
		keyData, err := fs.GetManagedKey(name)
		if err != nil {
			return err
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return fmt.Errorf("解析托管密钥失败: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else if h.PasswordRef != "" && fs != nil {
		pass, err := fs.GetPassword(h.PasswordRef)
		if err != nil {
			return err
		}
		authMethods = append(authMethods, ssh.Password(pass))
	} else {
		return fmt.Errorf("主机 %s 未配置认证凭据", h.Alias)
	}

	config := &ssh.ClientConfig{
		User:            h.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", h.Host, h.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	if direction == "push" {
		return pushFile(client, localPath, remotePath)
	}
	return pullFile(client, h.Alias, localPath, remotePath)
}

// pushFile uploads a file via SSH session stdin pipe.
func pushFile(client *ssh.Client, localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer f.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("创建会话失败: %w", err)
	}
	defer session.Close()

	remoteDir := filepath.Dir(remotePath)
	remoteFile := filepath.Base(remotePath)

	w, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建管道失败: %w", err)
	}

	if err := session.Start(fmt.Sprintf("mkdir -p %s && cat > %s/%s", remoteDir, remoteDir, remoteFile)); err != nil {
		return fmt.Errorf("启动远程命令失败: %w", err)
	}

	_, err = io.Copy(w, f)
	w.Close()
	if err != nil {
		return fmt.Errorf("传输文件失败: %w", err)
	}

	return session.Wait()
}

// pullFile downloads a file via SSH session stdout pipe, saved to localDir/hostalias/filename.
func pullFile(client *ssh.Client, hostAlias, localDir, remotePath string) error {
	destDir := filepath.Join(localDir, hostAlias)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("创建会话失败: %w", err)
	}
	defer session.Close()

	r, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("创建管道失败: %w", err)
	}

	remoteFile := filepath.Base(remotePath)
	destFile := filepath.Join(destDir, remoteFile)

	if err := session.Start(fmt.Sprintf("cat %s", remotePath)); err != nil {
		return fmt.Errorf("启动远程命令失败: %w", err)
	}

	f, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	if err != nil {
		return fmt.Errorf("接收文件失败: %w", err)
	}

	return session.Wait()
}

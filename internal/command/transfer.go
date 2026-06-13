package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v4/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/secret"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/v4/internal/ui"
	"github.com/pkg/sftp"
)

type transferOptions struct {
	direction  string
	localPath  string
	remotePath string
	targets    []string
	overwrite  bool
	yes        bool
	quiet      bool
}

func (app *App) cmdPush(args []string) error {
	options, err := parseTransferOptions(args, "push")
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func (app *App) cmdPull(args []string) error {
	options, err := parseTransferOptions(args, "pull")
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func parseTransferOptions(args []string, direction string) (transferOptions, error) {
	if len(args) < 3 {
		if direction == "push" {
			return transferOptions{}, fmt.Errorf("用法: sshm push <本地路径> <远程路径> <目标...> [--overwrite] [--yes] [--quiet]")
		}
		return transferOptions{}, fmt.Errorf("用法: sshm pull <远程路径> <本地目录> <目标...> [--overwrite] [--yes] [--quiet]")
	}
	options := transferOptions{direction: direction}
	if direction == "push" {
		options.localPath, options.remotePath = args[0], args[1]
	} else {
		options.remotePath, options.localPath = args[0], args[1]
	}
	for _, arg := range args[2:] {
		switch arg {
		case "--overwrite":
			options.overwrite = true
		case "--yes":
			options.yes = true
		case "--quiet":
			options.quiet = true
		default:
			options.targets = append(options.targets, arg)
		}
	}
	return options, nil
}

func (app *App) cmdTransfer(options transferOptions) error {
	hosts, err := app.selectHosts(options.targets)
	if err != nil {
		return err
	}
	needsConfirmation := options.direction == "push" || options.overwrite || len(hosts) > 1
	if needsConfirmation && !options.yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("该操作需要确认；非交互环境请显式使用 --yes")
		}
		fmt.Println()
		fmt.Printf("即将%s %d 台主机:\n", transferVerb(options.direction), len(hosts))
		for _, host := range hosts {
			fmt.Printf("  - %s (%s@%s:%d)\n", host.Alias, host.User, host.Host, host.Port)
		}
		fmt.Printf("  源: %s\n  目标: %s\n  覆盖: %t\n\n", transferSource(options), transferDestination(options), options.overwrite)
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	fs := app.tryGetSecretStore()
	results := make([]transferResult, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(4, len(hosts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
				method, destination, err := transferOne(ctx, hosts[i], fs, options)
				cancel()
				results[i] = transferResult{host: hosts[i], method: method, destination: destination, err: err, duration: time.Since(start)}
			}
		}()
	}
	for i := range hosts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	failed := 0
	logResults := make([]operation.Result, 0, len(results))
	for _, result := range results {
		retry := transferRetryCommand(options, result.host.Alias)
		opResult := newOperationResult(result.host,
			fmt.Sprintf("method=%s\ndestination=%s\n", result.method, result.destination),
			result.err, operation.StageTransfer, retry, result.duration)
		logResults = append(logResults, opResult)
		if result.err != nil {
			failed++
			printOperationFailure(opResult)
			continue
		}
		ui.PrintSuccess("%s: %s完成 [%s] -> %s", result.host.Alias, transferVerb(options.direction), result.method, result.destination)
	}
	fmt.Printf("%s完成：成功 %d，失败 %d\n", transferVerb(options.direction), len(results)-failed, failed)
	if len(results) > 1 || options.quiet {
		if err := writeOperationLog(options.direction+"-batch", transferSource(options)+" -> "+transferDestination(options), logResults); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("有 %d 台主机传输失败", failed)
	}
	return nil
}

type transferResult struct {
	host        config.Host
	method      string
	destination string
	err         error
	duration    time.Duration
}

func transferRetryCommand(options transferOptions, alias string) string {
	command := fmt.Sprintf("sshm %s %q %q %s --yes", options.direction, transferSource(options), transferDestination(options), alias)
	if options.overwrite {
		command += " --overwrite"
	}
	return command
}

func transferVerb(direction string) string {
	if direction == "pull" {
		return "拉取"
	}
	return "推送"
}

func transferSource(options transferOptions) string {
	if options.direction == "pull" {
		return options.remotePath
	}
	return options.localPath
}

func transferDestination(options transferOptions) string {
	if options.direction == "pull" {
		return options.localPath
	}
	return options.remotePath
}

func transferOne(ctx context.Context, host config.Host, store *secret.FileStore, options transferOptions) (string, string, error) {
	client, _, err := sshx.DialContext(ctx, host, store)
	if err != nil {
		return "sftp", "", err
	}
	defer client.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-done:
		}
	}()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return "sftp", "", fmt.Errorf("启动 SFTP 失败: %w", err)
	}
	defer sftpClient.Close()

	if destination, used, err := tryRsyncTransfer(ctx, client, sftpClient, host, store, options); used {
		return "rsync", destination, err
	}

	if options.direction == "push" {
		err = pushSFTP(sftpClient, options.localPath, options.remotePath, options.overwrite)
		return "sftp", options.remotePath, err
	}
	destination, err := pullSFTP(sftpClient, host.Alias, options.remotePath, options.localPath, options.overwrite)
	return "sftp", destination, err
}

func pushSFTP(client *sftp.Client, localPath, remotePath string, overwrite bool) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("读取本地源失败: %w", err)
	}
	remotePath = path.Clean(remotePath)
	if remotePath == "." || remotePath == "/" || strings.HasPrefix(remotePath, "~/") {
		return fmt.Errorf("远程目标必须是明确路径，且不支持 ~ 展开: %s", remotePath)
	}
	if _, err := client.Stat(remotePath); err == nil {
		if !overwrite {
			return fmt.Errorf("远程目标已存在；使用 --overwrite 明确覆盖")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查远程目标失败: %w", err)
	}
	temp := remotePath + fmt.Sprintf(".sshm-tmp-%d", time.Now().UnixNano())
	_ = client.RemoveAll(temp)
	defer client.RemoveAll(temp)
	if err := copyLocalToRemote(client, localPath, temp, info); err != nil {
		return err
	}
	return activateRemoteTemp(client, temp, remotePath, overwrite)
}

func copyLocalToRemote(client *sftp.Client, localPath, remotePath string, info os.FileInfo) error {
	if info.IsDir() {
		if err := client.MkdirAll(remotePath); err != nil {
			return fmt.Errorf("创建远程目录失败: %w", err)
		}
		return filepath.Walk(localPath, func(current string, entry os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(localPath, current)
			if err != nil || relative == "." {
				return err
			}
			target := path.Join(remotePath, filepath.ToSlash(relative))
			if entry.IsDir() {
				return client.MkdirAll(target)
			}
			return copyLocalFileToRemote(client, current, target, entry.Mode())
		})
	}
	if err := client.MkdirAll(path.Dir(remotePath)); err != nil {
		return err
	}
	return copyLocalFileToRemote(client, localPath, remotePath, info.Mode())
}

func copyLocalFileToRemote(client *sftp.Client, localPath, remotePath string, mode os.FileMode) error {
	source, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	if err := target.Chmod(mode.Perm()); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

func pullSFTP(client *sftp.Client, hostAlias, remotePath, localDir string, overwrite bool) (string, error) {
	remotePath = path.Clean(remotePath)
	info, err := client.Stat(remotePath)
	if err != nil {
		return "", fmt.Errorf("读取远程源失败: %w", err)
	}
	name := path.Base(remotePath)
	if name == "." || name == "/" {
		return "", fmt.Errorf("远程源必须是明确文件或目录")
	}
	destination := filepath.Join(localDir, hostAlias, name)
	if _, err := os.Stat(destination); err == nil {
		if !overwrite {
			return destination, fmt.Errorf("本地目标已存在；使用 --overwrite 明确覆盖")
		}
	} else if !os.IsNotExist(err) {
		return destination, fmt.Errorf("检查本地目标失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return destination, err
	}
	temp := destination + fmt.Sprintf(".sshm-tmp-%d", time.Now().UnixNano())
	_ = os.RemoveAll(temp)
	defer os.RemoveAll(temp)
	if err := copyRemoteToLocal(client, remotePath, temp, info); err != nil {
		return destination, err
	}
	return destination, activateLocalTemp(temp, destination, overwrite)
}

func copyRemoteToLocal(client *sftp.Client, remotePath, localPath string, info os.FileInfo) error {
	if info.IsDir() {
		if err := os.MkdirAll(localPath, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := client.ReadDir(remotePath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name, err := safeRemoteEntryName(entry.Name())
			if err != nil {
				return err
			}
			if err := copyRemoteToLocal(client, path.Join(remotePath, name), filepath.Join(localPath, name), entry); err != nil {
				return err
			}
		}
		return nil
	}
	source, err := client.Open(remotePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(localPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

func safeRemoteEntryName(name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || path.Base(name) != name {
		return "", fmt.Errorf("远程目录包含不安全名称: %q", name)
	}
	return name, nil
}

func activateRemoteTemp(client *sftp.Client, temp, destination string, overwrite bool) error {
	if !overwrite {
		if err := client.Rename(temp, destination); err != nil {
			return fmt.Errorf("启用远程目标失败: %w", err)
		}
		return nil
	}
	if _, err := client.Stat(destination); err != nil {
		if os.IsNotExist(err) {
			if err := client.Rename(temp, destination); err != nil {
				return fmt.Errorf("启用远程目标失败: %w", err)
			}
			return nil
		}
		return fmt.Errorf("检查远程目标失败: %w", err)
	}
	if err := client.PosixRename(temp, destination); err == nil {
		return nil
	}

	backup := destination + fmt.Sprintf(".sshm-restore-%d", time.Now().UnixNano())
	_ = client.RemoveAll(backup)
	if err := client.Rename(destination, backup); err != nil {
		return fmt.Errorf("准备远程目标替换失败，原目标未修改: %w", err)
	}
	if err := client.Rename(temp, destination); err != nil {
		restoreErr := client.Rename(backup, destination)
		if restoreErr != nil {
			return fmt.Errorf("启用远程目标失败且恢复原目标失败: %v；原始错误: %w", restoreErr, err)
		}
		return fmt.Errorf("启用远程目标失败，原目标已恢复: %w", err)
	}
	if err := client.RemoveAll(backup); err != nil {
		return fmt.Errorf("远程目标已替换，但清理恢复副本失败: %w", err)
	}
	return nil
}

func activateLocalTemp(temp, destination string, overwrite bool) error {
	if !overwrite {
		if err := os.Rename(temp, destination); err != nil {
			return fmt.Errorf("启用本地目标失败: %w", err)
		}
		return nil
	}
	if _, err := os.Stat(destination); err != nil {
		if os.IsNotExist(err) {
			if err := os.Rename(temp, destination); err != nil {
				return fmt.Errorf("启用本地目标失败: %w", err)
			}
			return nil
		}
		return fmt.Errorf("检查本地目标失败: %w", err)
	}

	backup := destination + fmt.Sprintf(".sshm-restore-%d", time.Now().UnixNano())
	_ = os.RemoveAll(backup)
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("准备本地目标替换失败，原目标未修改: %w", err)
	}
	if err := os.Rename(temp, destination); err != nil {
		restoreErr := os.Rename(backup, destination)
		if restoreErr != nil {
			return fmt.Errorf("启用本地目标失败且恢复原目标失败: %v；原始错误: %w", restoreErr, err)
		}
		return fmt.Errorf("启用本地目标失败，原目标已恢复: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("本地目标已替换，但清理恢复副本失败: %w", err)
	}
	return nil
}

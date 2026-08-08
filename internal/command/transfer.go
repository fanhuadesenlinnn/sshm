package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type transferOptions struct {
	direction        string
	localPath        string
	remotePath       string
	targets          []string
	batch            batchCLIOptions
	overwrite        bool
	backup           bool
	validateChecksum bool
	flat             bool
	method           string
	connectTimeout   time.Duration
	destinationExact bool
	check            bool
	diffWriter       io.Writer
	multi            bool
}

func (app *App) cmdPush(args []string) error {
	options, err := parseTransferOptions(args, "push", false)
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func (app *App) cmdPull(args []string) error {
	options, err := parseTransferOptions(args, "pull", false)
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func (app *App) cmdPushTag(args []string) error {
	options, err := parseTransferOptions(args, "push", true)
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func (app *App) cmdPullTag(args []string) error {
	options, err := parseTransferOptions(args, "pull", true)
	if err != nil {
		return err
	}
	return app.cmdTransfer(options)
}

func parseTransferOptions(args []string, direction string, tag bool) (transferOptions, error) {
	options := transferOptions{direction: direction, method: "auto", validateChecksum: true, multi: tag}
	var commonArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			commonArgs = append(commonArgs, args[i:]...)
			break
		}
		switch args[i] {
		case "--overwrite":
			options.overwrite = true
		case "--backup":
			options.backup = true
		case "--no-validate-checksum":
			options.validateChecksum = false
		case "--flat":
			options.flat = true
		case "--method":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--method 缺少值")
			}
			i++
			options.method = args[i]
		default:
			commonArgs = append(commonArgs, args[i])
		}
	}
	batchOptions, positionals, err := parseBatchCLIOptions(commonArgs)
	if err != nil {
		return options, err
	}
	options.batch = batchOptions
	if options.overwrite && options.backup {
		return options, fmt.Errorf("--overwrite 与 --backup 不能同时使用")
	}
	if options.flat && direction != "pull" {
		return options, fmt.Errorf("--flat 仅适用于 pull")
	}
	if options.method != "auto" && options.method != "sftp" && options.method != "rsync" {
		return options, fmt.Errorf("--method 必须是 auto、sftp 或 rsync")
	}
	if len(positionals) != 3 {
		target := "<host>"
		command := direction
		if tag {
			target = "<tag>"
			command += "-tag"
		}
		if direction == "push" {
			return options, fmt.Errorf("用法: sshmd %s %s <local> <remote> [选项]", command, target)
		}
		return options, fmt.Errorf("用法: sshmd %s %s <remote> <local> [选项]", command, target)
	}
	if tag {
		if positionals[0] == "all" {
			options.targets = []string{"--all"}
		} else {
			options.targets = []string{"--tag", positionals[0]}
		}
	} else {
		options.targets = []string{positionals[0]}
	}
	if direction == "push" {
		options.localPath, options.remotePath = positionals[1], positionals[2]
	} else {
		options.remotePath, options.localPath = positionals[1], positionals[2]
	}
	return options, nil
}

func (app *App) cmdTransfer(options transferOptions) error {
	hosts, err := app.selectHosts(options.targets)
	if err != nil {
		return err
	}
	if len(options.batch.Exclude) > 0 || len(options.batch.ExcludeTags) > 0 {
		hf, loadErr := app.Store.Load()
		if loadErr != nil {
			return loadErr
		}
		hosts, err = deploy.ApplyExcludes(hosts, hf.Hosts, options.batch.Exclude, options.batch.ExcludeTags)
		if err != nil {
			return err
		}
	}
	options.multi = options.multi || len(hosts) > 1
	if options.direction == "push" {
		if _, err := localManifest(options.localPath); err != nil {
			return err
		}
	}

	destinations := map[string]string{}
	if options.direction == "pull" && options.multi {
		var paths []string
		for _, host := range hosts {
			destination, err := multiPullDestination(options.localPath, host.Alias, options.remotePath, options.flat)
			if err != nil {
				return err
			}
			destinations[host.ID] = destination
			paths = append(paths, destination)
		}
		if err := ensureUniqueDestinations(paths, localPathComparisonCaseInsensitive()); err != nil {
			return err
		}
	}

	needsConfirmation := options.multi || options.direction == "push" || options.overwrite || options.backup
	if needsConfirmation && !options.batch.Yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("该操作需要确认；非交互环境请显式使用 --yes")
		}
		fmt.Printf("即将%s %d 台主机:\n", transferVerb(options.direction), len(hosts))
		for _, host := range hosts {
			fmt.Printf("  - %s (%s@%s:%d)\n", host.Alias, host.User, host.Host, host.Port)
		}
		fmt.Printf("源: %s\n目标: %s\n覆盖: %t\n备份: %t\n", transferSource(options), transferDestination(options), options.overwrite, options.backup)
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}
	if err := app.unlockVaultForHosts(hosts); err != nil {
		return &ExitError{Code: 4, Err: err}
	}
	batchOptions, timeout, connectTimeout, logDefaults, err := app.resolveBatchOptions(options.batch, true)
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}
	executor := app.operationExecutor()
	defer executor.CloseSessions()
	ctx, cancel := signalContext()
	defer cancel()
	runner := batch.Runner{Options: batchOptions}
	runResult, err := runner.Run(ctx, hosts, func(ctx context.Context, host config.Host) batch.Result {
		taskCtx, taskCancel := context.WithTimeout(ctx, timeout)
		defer taskCancel()
		source := transferSource(options)
		destination := transferDestination(options)
		destinationExact := false
		if options.direction == "pull" && options.multi {
			destination = destinations[host.ID]
			destinationExact = true
		}
		transfer := ops.TransferOptions{
			Direction: options.direction, Src: source, Dest: destination, Method: options.method,
			Overwrite: options.overwrite, Backup: options.backup, ValidateChecksum: options.validateChecksum,
			DestinationExact: destinationExact, ConnectTimeout: connectTimeout,
		}
		var opResult ops.Result
		if options.direction == "push" {
			opResult = executor.Push(taskCtx, host, transfer)
		} else {
			opResult = executor.Pull(taskCtx, host, transfer)
		}
		status := batchStatus(opResult)
		if opResult.Err == nil && opResult.Changed {
			status = batch.StatusChanged
		}
		return batch.Result{Status: status, Err: opResult.Err, Detail: opResult.Destination, Value: opResult}
	})
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}

	logResults := make([]operation.Result, 0, len(runResult.Results))
	for _, result := range runResult.Results {
		if result.Status == batch.StatusSkipped {
			ui.PrintWarn("%s: skipped (%s)", result.Host.Alias, result.SkippedReason)
			logResults = append(logResults, skippedOperationResult(result.Host, result.SkippedReason))
			continue
		}
		opResult := result.Value.(ops.Result)
		logResult := newOperationResult(result.Host,
			fmt.Sprintf("method=%s\ndestination=%s\nstatus=%s\n", opResult.Method, opResult.Destination, result.Status),
			opResult.Err, operation.StageTransfer, transferRetryCommandForError(options, result.Host.Alias, opResult.Err), opResult.Duration)
		logResults = append(logResults, logResult)
		if opResult.Err != nil {
			printOperationFailure(logResult)
			continue
		}
		fmt.Printf("%s  %-8s [%s] -> %s\n", result.Host.Alias, result.Status, opResult.Method, opResult.Destination)
	}
	printBatchSummary(runResult.Summary)
	if logDefaults.Enabled && !options.batch.NoLog {
		if err := writeOperationLog(options.direction+"-batch", transferSource(options)+" -> "+transferDestination(options), logResults); err != nil {
			return err
		}
	}
	return exitErrorForBatch(runResult)
}

func transferRetryCommand(options transferOptions, alias string) string {
	command := fmt.Sprintf("sshmd %s %s %s %s --yes",
		options.direction,
		shellquote.Single(alias),
		shellquote.Single(transferSource(options)),
		shellquote.Single(transferDestination(options)),
	)
	if options.overwrite {
		command += " --overwrite"
	}
	if options.backup {
		command += " --backup"
	}
	if !options.validateChecksum {
		command += " --no-validate-checksum"
	}
	if options.method != "" && options.method != "auto" {
		command += " --method " + shellquote.Single(options.method)
	}
	return command
}

func transferRetryCommandForError(options transferOptions, alias string, err error) string {
	if err != nil && !options.overwrite && !options.backup && strings.Contains(err.Error(), "使用 --overwrite 或 --backup") {
		options.backup = true
	}
	return transferRetryCommand(options, alias)
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

func transferOne(ctx context.Context, host config.Host, store *secret.FileStore, options transferOptions) (string, string, bool, error) {
	dialCtx := ctx
	cancelDial := func() {}
	if options.connectTimeout > 0 {
		dialCtx, cancelDial = context.WithTimeout(ctx, options.connectTimeout)
	}
	client, _, err := sshx.DialContextWithTimeout(dialCtx, host, store, options.connectTimeout)
	cancelDial()
	if err != nil {
		return "sftp", "", false, err
	}
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
		_ = client.Close()
		return "sftp", "", false, fmt.Errorf("启动 SFTP 失败: %w", err)
	}
	return transferWithClients(ctx, host, store, options, client, sftpClient, func() {
		_ = sftpClient.Close()
		_ = client.Close()
	})
}

// transferWithClients performs the transfer over pre-established clients.
// closeFn may be nil when the clients are owned by a reusable session and
// must stay open for later operations.
func transferWithClients(
	ctx context.Context,
	host config.Host,
	store *secret.FileStore,
	options transferOptions,
	client *ssh.Client,
	sftpClient *sftp.Client,
	closeFn func(),
) (string, string, bool, error) {
	if closeFn != nil {
		defer closeFn()
	}
	if options.method == "" {
		options.method = "auto"
	}
	destination := options.localPath
	var err error
	if options.direction == "pull" && !options.destinationExact {
		destination, err = singlePullDestination(options.remotePath, options.localPath)
		if err != nil {
			return "sftp", "", false, err
		}
		options.localPath = destination
		options.destinationExact = true
	}
	if options.method != "sftp" {
		rsyncDestination, changed, used, err := tryRsyncTransfer(ctx, client, sftpClient, host, store, options)
		if used {
			return "rsync", rsyncDestination, changed, err
		}
	}
	if options.direction == "push" {
		changed, err := pushSFTP(sftpClient, options.localPath, options.remotePath, options)
		return "sftp", options.remotePath, changed, err
	}
	changed, err := pullSFTP(sftpClient, options.remotePath, destination, options)
	return "sftp", destination, changed, err
}

func cleanTransferRemotePath(remotePath string) (string, error) {
	if remotePath == "" || strings.ContainsRune(remotePath, '\x00') {
		return "", fmt.Errorf("远程路径不能为空或包含 NUL")
	}
	if remotePath == "~" || strings.HasPrefix(remotePath, "~/") {
		return "", fmt.Errorf("远程路径必须是明确路径，且不支持 ~ 展开: %s", remotePath)
	}
	for _, component := range strings.Split(remotePath, "/") {
		if component == ".." {
			return "", fmt.Errorf("远程路径不能包含上级目录组件: %s", remotePath)
		}
	}
	clean := path.Clean(remotePath)
	if clean == "." || clean == "/" {
		return "", fmt.Errorf("远程路径必须指向明确文件或目录: %s", remotePath)
	}
	return clean, nil
}

func pushSFTP(client *sftp.Client, localPath, remotePath string, options transferOptions) (bool, error) {
	sourceManifest, err := localManifest(localPath)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(localPath)
	if err != nil {
		return false, fmt.Errorf("读取本地源失败: %w", err)
	}
	remotePath, err = cleanTransferRemotePath(remotePath)
	if err != nil {
		return false, err
	}
	exists := false
	var targetManifest []manifestEntry
	if _, err := client.Lstat(remotePath); err == nil {
		exists = true
		if options.validateChecksum {
			targetManifest, err = remoteManifest(client, remotePath)
			if err != nil {
				return false, err
			}
			if manifestsEqual(sourceManifest, targetManifest) {
				return false, nil
			}
		}
		if options.diffWriter != nil {
			if targetManifest == nil {
				targetManifest, err = remoteManifest(client, remotePath)
				if err != nil {
					return false, err
				}
			}
			if err := writePushDiff(options.diffWriter, client, localPath, remotePath, sourceManifest, targetManifest); err != nil {
				return false, err
			}
		}
		if !options.check && !options.overwrite && !options.backup {
			return false, fmt.Errorf("远程目标已存在且内容不同；使用 --overwrite 或 --backup")
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("检查远程目标失败: %w", err)
	} else if options.diffWriter != nil {
		if err := writePushDiff(options.diffWriter, client, localPath, remotePath, sourceManifest, nil); err != nil {
			return false, err
		}
	}
	if options.check {
		return true, nil
	}
	temp := remotePath + fmt.Sprintf(".sshmd-tmp-%d", time.Now().UnixNano())
	_ = client.RemoveAll(temp)
	defer client.RemoveAll(temp)
	if err := copyLocalToRemote(client, localPath, temp, info); err != nil {
		return false, err
	}
	if options.validateChecksum {
		tempManifest, err := remoteManifest(client, temp)
		if err != nil {
			return false, err
		}
		if !manifestsEqual(sourceManifest, tempManifest) {
			return false, fmt.Errorf("远程临时目标 checksum 校验失败")
		}
	}
	return true, activateRemoteTemp(client, temp, remotePath, exists, options.overwrite, options.backup)
}

func pullSFTP(client *sftp.Client, remotePath, destination string, options transferOptions) (bool, error) {
	var err error
	remotePath, err = cleanTransferRemotePath(remotePath)
	if err != nil {
		return false, err
	}
	sourceManifest, err := remoteManifest(client, remotePath)
	if err != nil {
		return false, err
	}
	if err := validateRemoteManifestDestinations(
		destination, sourceManifest, runtime.GOOS == "windows", localPathComparisonCaseInsensitive(),
	); err != nil {
		return false, err
	}
	info, err := client.Lstat(remotePath)
	if err != nil {
		return false, fmt.Errorf("读取远程源失败: %w", err)
	}
	exists := false
	if _, err := os.Lstat(destination); err == nil {
		exists = true
		if options.validateChecksum {
			targetManifest, err := localManifest(destination)
			if err != nil {
				return false, err
			}
			if manifestsEqual(sourceManifest, targetManifest) {
				return false, nil
			}
		}
		if !options.check && !options.overwrite && !options.backup {
			return false, fmt.Errorf("本地目标已存在且内容不同；使用 --overwrite 或 --backup")
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("检查本地目标失败: %w", err)
	}
	if options.check {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return false, err
	}
	temp := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".sshmd-tmp-"+fmt.Sprint(time.Now().UnixNano()))
	_ = os.RemoveAll(temp)
	defer os.RemoveAll(temp)
	if err := copyRemoteToLocal(client, remotePath, temp, info); err != nil {
		return false, err
	}
	if options.validateChecksum {
		tempManifest, err := localManifest(temp)
		if err != nil {
			return false, err
		}
		if !manifestsEqual(sourceManifest, tempManifest) {
			return false, fmt.Errorf("本地临时目标 checksum 校验失败")
		}
	}
	return true, activateLocalTemp(temp, destination, exists, options.overwrite, options.backup)
}

func singlePullDestination(remotePath, localPath string) (string, error) {
	parts, err := safeRemotePathParts(remotePath, runtime.GOOS == "windows")
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(localPath); err == nil && info.IsDir() {
		return confinedJoin(localPath, parts[len(parts)-1])
	}
	if strings.HasSuffix(localPath, string(os.PathSeparator)) || strings.HasSuffix(localPath, "/") {
		return confinedJoin(localPath, parts[len(parts)-1])
	}
	absolute, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func copyLocalToRemote(client *sftp.Client, localPath, remotePath string, info os.FileInfo) error {
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("不支持符号链接或特殊文件: %s", localPath)
	}
	if info.IsDir() {
		if err := client.MkdirAll(remotePath); err != nil {
			return fmt.Errorf("创建远程目录失败: %w", err)
		}
		if err := client.Chmod(remotePath, info.Mode().Perm()); err != nil {
			return err
		}
		children, err := os.ReadDir(localPath)
		if err != nil {
			return err
		}
		for _, child := range children {
			current := filepath.Join(localPath, child.Name())
			childInfo, err := os.Lstat(current)
			if err != nil {
				return err
			}
			if err := copyLocalToRemote(client, current, path.Join(remotePath, child.Name()), childInfo); err != nil {
				return err
			}
		}
		return nil
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
		_ = target.Close()
		return err
	}
	if err := target.Chmod(mode.Perm()); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}

func copyRemoteToLocal(client *sftp.Client, remotePath, localPath string, info os.FileInfo) error {
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("不支持远程符号链接或特殊文件: %s", remotePath)
	}
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
			if err := validateLocalComponent(name, runtime.GOOS == "windows"); err != nil {
				return fmt.Errorf("远程目录项 %q 无法安全保存: %w", name, err)
			}
			childInfo, err := client.Lstat(path.Join(remotePath, name))
			if err != nil {
				return err
			}
			if err := copyRemoteToLocal(client, path.Join(remotePath, name), filepath.Join(localPath, name), childInfo); err != nil {
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
		_ = target.Close()
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

func activateRemoteTemp(client *sftp.Client, temp, destination string, exists, overwrite, backup bool) error {
	if !exists {
		return client.Rename(temp, destination)
	}
	if backup {
		backupPath := destination + ".bak." + time.Now().Format("20060102-150405")
		if _, err := client.Lstat(backupPath); err == nil {
			backupPath += fmt.Sprintf("-%d", time.Now().UnixNano())
		}
		if err := client.Rename(destination, backupPath); err != nil {
			return fmt.Errorf("备份远程目标失败: %w", err)
		}
		if err := client.Rename(temp, destination); err != nil {
			_ = client.Rename(backupPath, destination)
			return fmt.Errorf("启用远程目标失败，已尝试恢复备份: %w", err)
		}
		return nil
	}
	if !overwrite {
		return fmt.Errorf("远程目标已存在")
	}
	if err := client.PosixRename(temp, destination); err == nil {
		return nil
	}
	restore := destination + fmt.Sprintf(".sshmd-restore-%d", time.Now().UnixNano())
	if err := client.Rename(destination, restore); err != nil {
		return fmt.Errorf("准备远程目标替换失败: %w", err)
	}
	if err := client.Rename(temp, destination); err != nil {
		_ = client.Rename(restore, destination)
		return fmt.Errorf("启用远程目标失败，已尝试恢复原目标: %w", err)
	}
	return client.RemoveAll(restore)
}

func activateLocalTemp(temp, destination string, exists, overwrite, backup bool) error {
	if !exists {
		return os.Rename(temp, destination)
	}
	if backup {
		backupPath := destination + ".bak." + time.Now().Format("20060102-150405")
		if _, err := os.Lstat(backupPath); err == nil {
			backupPath += fmt.Sprintf("-%d", time.Now().UnixNano())
		}
		if err := os.Rename(destination, backupPath); err != nil {
			return fmt.Errorf("备份本地目标失败: %w", err)
		}
		if err := os.Rename(temp, destination); err != nil {
			_ = os.Rename(backupPath, destination)
			return fmt.Errorf("启用本地目标失败，已尝试恢复备份: %w", err)
		}
		return nil
	}
	if !overwrite {
		return fmt.Errorf("本地目标已存在")
	}
	restore := destination + fmt.Sprintf(".sshmd-restore-%d", time.Now().UnixNano())
	if err := os.Rename(destination, restore); err != nil {
		return fmt.Errorf("准备本地目标替换失败: %w", err)
	}
	if err := os.Rename(temp, destination); err != nil {
		_ = os.Rename(restore, destination)
		return fmt.Errorf("启用本地目标失败，已尝试恢复原目标: %w", err)
	}
	return os.RemoveAll(restore)
}

package deploy

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
)

type unarchiveArgs struct {
	Src     string `yaml:"src"`
	Dest    string `yaml:"dest"`
	Creates string `yaml:"creates,omitempty"`
	Backup  bool   `yaml:"backup,omitempty"`
}

type unarchiveModule struct{}

const (
	maxArchiveEntries       = 200_000
	maxArchiveUnpackedBytes = int64(32 << 30)
	maxArchiveSingleFile    = int64(16 << 30)
	maxArchiveExpansion     = int64(1_000)
)

func (m *unarchiveModule) Name() string {
	return "unarchive"
}

func (m *unarchiveModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args unarchiveArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Src) == "" || strings.TrimSpace(args.Dest) == "" {
		return nil, fmt.Errorf("unarchive 需要 src 和 dest")
	}
	if err := validateUnarchiveDestination(args.Dest); err != nil {
		return nil, err
	}
	args.Dest = path.Clean(args.Dest)
	return &args, nil
}

func validateUnarchiveDestination(destination string) error {
	if strings.ContainsRune(destination, '\x00') || destination == "~" || strings.HasPrefix(destination, "~/") {
		return fmt.Errorf("unarchive dest 必须是明确路径，且不支持 NUL 或 ~ 展开")
	}
	clean := path.Clean(destination)
	if clean == "." || clean == "/" || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("unarchive dest 必须指向非根目录，且不能包含上级目录")
	}
	for _, component := range strings.Split(destination, "/") {
		if component == ".." {
			return fmt.Errorf("unarchive dest 不能包含上级目录组件")
		}
	}
	return nil
}

func (m *unarchiveModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*unarchiveArgs)
	if args.Creates != "" {
		exists, err := remoteTest(tc, args.Creates)
		if err != nil {
			return failedModule(err, operation.StageExecute)
		}
		if exists {
			return ModuleResult{Status: batch.StatusOK, Output: "creates 已存在: " + args.Creates + "\n"}
		}
	}
	projectRoot := tc.ProjectRoot
	if projectRoot == "" {
		projectRoot = tc.BaseDir
	}
	localPath, pathErr := resolveProjectPath(projectRoot, tc.BaseDir, args.Src)
	if pathErr != nil {
		return failedModule(fmt.Errorf("unarchive src 路径无效: %w", pathErr), operation.StageConfig)
	}
	if _, err := os.Stat(localPath); err != nil {
		return failedModule(fmt.Errorf("本地压缩包不存在: %s", localPath), operation.StageConfig)
	}
	kind := archiveKind(localPath)
	if kind.extension == "" {
		return failedModule(fmt.Errorf("无法识别的压缩包格式: %s（支持 .tar.gz/.tgz/.zip）", localPath), operation.StageConfig)
	}
	if err := validateLocalArchive(localPath, kind); err != nil {
		return failedModule(fmt.Errorf("压缩包安全校验失败: %w", err), operation.StageConfig)
	}
	tempDirResult := runRemote(tc, "mktemp -d -t sshmd-unarchive-XXXXXX")
	if tempDirResult.Status != batch.StatusOK {
		return tempDirResult
	}
	tempDir := strings.TrimSpace(tempDirResult.Output)
	defer m.cleanup(tc, tempDir)
	archivePath := tempDir + "/archive" + kind.extension
	push := tc.Executor.Push(tc.Ctx, tc.Host, ops.TransferOptions{
		Direction: "push", Src: localPath, Dest: archivePath, Method: "auto",
		Overwrite: true, ValidateChecksum: true, ConnectTimeout: tc.ConnectTimeout,
	})
	if push.Err != nil {
		return failedModule(push.Err, push.Stage)
	}
	extractDir := tempDir + "/extract"
	mkdir := runRemote(tc, "mkdir -p -- "+shellquote.Single(extractDir))
	if mkdir.Status != batch.StatusOK {
		return mkdir
	}
	extract := runRemote(tc, kind.extractCommand(archivePath, extractDir))
	if extract.Status != batch.StatusOK {
		return extract
	}
	destInfo, err := statResult(tc, args.Dest)
	if err != nil {
		return failedModule(err, operation.StageTransfer)
	}
	if destInfo.Exists && !destInfo.IsDir {
		return failedModule(fmt.Errorf("unarchive dest 已存在但不是目录: %s", args.Dest), operation.StageExecute)
	}
	same, err := m.treeEqual(tc, extractDir, args.Dest)
	if err != nil {
		return failedModule(err, operation.StageExecute)
	}
	if destInfo.Exists && same {
		return ModuleResult{Status: batch.StatusOK, Output: "解压内容与目标一致\n"}
	}
	if tc.Check {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true,
			Output: "解压 " + localPath + " 到 " + args.Dest + "\n"}
	}
	parent := path.Dir(args.Dest)
	mkdirParent := runRemote(tc, "mkdir -p -- "+shellquote.Single(parent))
	if mkdirParent.Status != batch.StatusOK {
		return mkdirParent
	}
	stageResult := runRemote(tc, "mktemp -d "+shellquote.Single(path.Join(parent, ".sshmd-unarchive-stage-XXXXXX")))
	if stageResult.Status != batch.StatusOK {
		return stageResult
	}
	stageDir := strings.TrimSpace(stageResult.Output)
	defer m.cleanup(tc, stageDir)
	stage := runRemote(tc, "cp -a "+shellquote.Single(extractDir+"/.")+" "+shellquote.Single(stageDir))
	if stage.Status != batch.StatusOK {
		return stage
	}
	backupOutput := ""
	if destInfo.Exists {
		oldPath := args.Dest + ".sshmd-restore-" + fmt.Sprintf("%d", time.Now().UnixNano())
		if args.Backup {
			oldPath = args.Dest + ".bak." + time.Now().Format("20060102-150405") + fmt.Sprintf("-%d", time.Now().UnixNano())
			backupOutput = "已备份原目录到 " + oldPath + "\n"
		}
		switchCommand := "set -e; mv -- " + shellquote.Single(args.Dest) + " " + shellquote.Single(oldPath) +
			"; if mv -- " + shellquote.Single(stageDir) + " " + shellquote.Single(args.Dest) +
			"; then "
		if args.Backup {
			switchCommand += ":"
		} else {
			switchCommand += "rm -rf -- " + shellquote.Single(oldPath)
		}
		switchCommand += "; else mv -- " + shellquote.Single(oldPath) + " " + shellquote.Single(args.Dest) + "; exit 1; fi"
		switchResult := runRemote(tc, switchCommand)
		if switchResult.Status != batch.StatusOK {
			return switchResult
		}
	} else {
		activate := runRemote(tc, "mv -- "+shellquote.Single(stageDir)+" "+shellquote.Single(args.Dest))
		if activate.Status != batch.StatusOK {
			return activate
		}
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true,
		Output: backupOutput + "已解压 " + localPath + " 到 " + args.Dest + "\n"}
}

func (m *unarchiveModule) cleanup(tc TaskContext, tempDir string) {
	if tempDir == "" || strings.ContainsRune(tempDir, '\n') {
		return
	}
	runRemote(tc, "rm -rf -- "+shellquote.Single(tempDir))
}

// treeEqual compares the sha256 of every file under two remote directories.
func (m *unarchiveModule) treeEqual(tc TaskContext, left, right string) (bool, error) {
	leftInfo, err := statResult(tc, left)
	if err != nil {
		return false, err
	}
	rightInfo, err := statResult(tc, right)
	if err != nil {
		return false, err
	}
	if !leftInfo.Exists || !rightInfo.Exists {
		return false, nil
	}
	leftHashes, err := remoteTreeHashes(tc, left)
	if err != nil {
		return false, err
	}
	rightHashes, err := remoteTreeHashes(tc, right)
	if err != nil {
		return false, err
	}
	if len(leftHashes) != len(rightHashes) {
		return false, nil
	}
	for path, hash := range leftHashes {
		if rightHashes[path] != hash {
			return false, nil
		}
	}
	return true, nil
}

func remoteTreeHashes(tc TaskContext, dir string) (map[string]string, error) {
	bin := runRemote(tc, "(command -v sha256sum || command -v shasum) 2>/dev/null | head -1")
	if bin.Status != batch.StatusOK {
		return nil, fmt.Errorf("远端缺少 sha256sum/shasum")
	}
	name := strings.TrimSpace(bin.Output)
	flag := ""
	if strings.HasSuffix(name, "shasum") {
		flag = "-a 256"
	}
	result := runRemote(tc, "cd "+shellquote.Single(dir)+" && find . -type f -print0 | sort -z | xargs -0 "+name+" "+flag)
	if result.Status != batch.StatusOK {
		reason := result.Output
		if result.Err != nil {
			reason = result.Err.Error()
		}
		return nil, fmt.Errorf("计算远端校验和失败: %s", reason)
	}
	hashes := map[string]string{}
	for _, line := range strings.Split(result.Output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(fields[len(fields)-1], "./"), "./")
		hashes[relative] = fields[0]
	}
	return hashes, nil
}

type archiveKindInfo struct {
	extension      string
	extractCommand func(path, dir string) string
}

func archiveKind(path string) archiveKindInfo {
	switch {
	case strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tgz"):
		return archiveKindInfo{
			extension: ".tar.gz",
			extractCommand: func(p, dir string) string {
				return "tar -xzf " + shellquote.Single(p) + " -C " + shellquote.Single(dir)
			},
		}
	case strings.HasSuffix(path, ".zip"):
		return archiveKindInfo{
			extension: ".zip",
			extractCommand: func(p, dir string) string {
				return "unzip -q " + shellquote.Single(p) + " -d " + shellquote.Single(dir)
			},
		}
	default:
		return archiveKindInfo{}
	}
}

func validateLocalArchive(file string, kind archiveKindInfo) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	var entries int
	var unpacked int64
	add := func(name string, size int64, mode os.FileMode, regular, directory bool) error {
		if err := validateArchivePath(name); err != nil {
			return fmt.Errorf("条目 %q: %w", name, err)
		}
		if !regular && !directory {
			return fmt.Errorf("条目 %q 是符号链接、硬链接或特殊文件", name)
		}
		if mode&(os.ModeSetuid|os.ModeSetgid) != 0 {
			return fmt.Errorf("条目 %q 包含 setuid/setgid 权限", name)
		}
		if size < 0 || size > maxArchiveSingleFile {
			return fmt.Errorf("条目 %q 解压后大小超过 %d GiB", name, maxArchiveSingleFile>>30)
		}
		entries++
		if entries > maxArchiveEntries {
			return fmt.Errorf("条目数量超过上限 %d", maxArchiveEntries)
		}
		if size > maxArchiveUnpackedBytes-unpacked {
			return fmt.Errorf("解压后总大小超过 %d GiB", maxArchiveUnpackedBytes>>30)
		}
		unpacked += size
		return nil
	}

	switch kind.extension {
	case ".tar.gz":
		input, err := os.Open(file)
		if err != nil {
			return err
		}
		defer input.Close()
		compressed, err := gzip.NewReader(input)
		if err != nil {
			return fmt.Errorf("读取 gzip 失败: %w", err)
		}
		defer compressed.Close()
		reader := tar.NewReader(compressed)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("读取 tar 失败: %w", err)
			}
			regular := header.Typeflag == tar.TypeReg || header.Typeflag == 0
			directory := header.Typeflag == tar.TypeDir
			if err := add(header.Name, header.Size, header.FileInfo().Mode(), regular, directory); err != nil {
				return err
			}
		}
	case ".zip":
		reader, err := zip.OpenReader(file)
		if err != nil {
			return fmt.Errorf("读取 zip 失败: %w", err)
		}
		defer reader.Close()
		for _, entry := range reader.File {
			if entry.Flags&1 != 0 {
				return fmt.Errorf("条目 %q 使用加密 zip，拒绝部署", entry.Name)
			}
			if entry.UncompressedSize64 > uint64(maxArchiveSingleFile) {
				return fmt.Errorf("条目 %q 解压后大小超过 %d GiB", entry.Name, maxArchiveSingleFile>>30)
			}
			mode := entry.Mode()
			if err := add(entry.Name, int64(entry.UncompressedSize64), mode, mode.IsRegular(), mode.IsDir()); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("不支持的压缩包格式")
	}
	if info.Size() > 0 && unpacked > 1<<30 && unpacked/info.Size() > maxArchiveExpansion {
		return fmt.Errorf("解压膨胀比例超过上限 %d:1", maxArchiveExpansion)
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("路径为空或包含 NUL")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("绝对路径")
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("包含上级目录组件")
	}
	for _, component := range strings.Split(name, "/") {
		if component == ".." {
			return fmt.Errorf("包含上级目录组件")
		}
	}
	return nil
}

package deploy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/shellquote"
)

type unarchiveArgs struct {
	Src     string `yaml:"src"`
	Dest    string `yaml:"dest"`
	Creates string `yaml:"creates,omitempty"`
	Backup  bool   `yaml:"backup,omitempty"`
}

type unarchiveModule struct{}

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
	return &args, nil
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
	localPath := resolveRelative(tc.BaseDir, args.Src)
	if _, err := os.Stat(localPath); err != nil {
		return failedModule(fmt.Errorf("本地压缩包不存在: %s", localPath), operation.StageConfig)
	}
	kind := archiveKind(localPath)
	if kind.extension == "" {
		return failedModule(fmt.Errorf("无法识别的压缩包格式: %s（支持 .tar.gz/.tgz/.zip）", localPath), operation.StageConfig)
	}
	tempDirResult := runRemote(tc, "mktemp -d -t sshm-unarchive-XXXXXX")
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
	listing := runRemote(tc, kind.listCommand(archivePath))
	if listing.Status != batch.StatusOK {
		return listing
	}
	entries := strings.Split(listing.Output, "\n")
	for _, entry := range entries {
		if err := validateArchiveEntry(entry); err != nil {
			return failedModule(fmt.Errorf("压缩包包含不安全条目 %q: %w", entry, err), operation.StageConfig)
		}
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
	mkdirDest := runRemote(tc, "mkdir -p -- "+shellquote.Single(args.Dest))
	if mkdirDest.Status != batch.StatusOK {
		return mkdirDest
	}
	sync := runRemote(tc, "cp -a "+shellquote.Single(extractDir+"/.")+" "+shellquote.Single(args.Dest))
	if sync.Status != batch.StatusOK {
		return sync
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true,
		Output: "已解压 " + localPath + " 到 " + args.Dest + "\n"}
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
	listCommand    func(path string) string
	extractCommand func(path, dir string) string
}

func archiveKind(path string) archiveKindInfo {
	switch {
	case strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tgz"):
		return archiveKindInfo{
			extension: ".tar.gz",
			listCommand: func(p string) string {
				return "tar -tzf " + shellquote.Single(p)
			},
			extractCommand: func(p, dir string) string {
				return "tar -xzf " + shellquote.Single(p) + " -C " + shellquote.Single(dir)
			},
		}
	case strings.HasSuffix(path, ".zip"):
		return archiveKindInfo{
			extension: ".zip",
			listCommand: func(p string) string {
				return "unzip -Z1 " + shellquote.Single(p)
			},
			extractCommand: func(p, dir string) string {
				return "unzip -q " + shellquote.Single(p) + " -d " + shellquote.Single(dir)
			},
		}
	default:
		return archiveKindInfo{}
	}
}

func validateArchiveEntry(entry string) error {
	entry = strings.TrimSuffix(entry, "/")
	if entry == "" {
		return nil
	}
	if strings.HasPrefix(entry, "/") {
		return fmt.Errorf("绝对路径")
	}
	for _, component := range strings.Split(entry, "/") {
		if component == ".." {
			return fmt.Errorf("包含上级目录组件")
		}
	}
	if index := strings.Index(entry, " -> "); index >= 0 {
		target := strings.TrimSpace(entry[index+4:])
		if strings.HasPrefix(target, "/") {
			return fmt.Errorf("软链目标为绝对路径")
		}
		for _, component := range strings.Split(target, "/") {
			if component == ".." {
				return fmt.Errorf("软链目标包含上级目录组件")
			}
		}
	}
	return nil
}

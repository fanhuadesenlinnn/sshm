package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
)

type fetchArgs struct {
	Src    string `yaml:"src"`
	Dest   string `yaml:"dest"`
	Flat   bool   `yaml:"flat,omitempty"`
	Backup bool   `yaml:"backup,omitempty"`
}

type fetchModule struct{}

func (m *fetchModule) Name() string {
	return "fetch"
}

func (m *fetchModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args fetchArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Src) == "" || strings.TrimSpace(args.Dest) == "" {
		return nil, fmt.Errorf("fetch 需要 src 和 dest")
	}
	return &args, nil
}

func (m *fetchModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*fetchArgs)
	projectRoot := tc.ProjectRoot
	if projectRoot == "" {
		projectRoot = tc.BaseDir
	}
	destination, err := pullDestination(projectRoot, tc.BaseDir, tc.Host.Alias, args.Dest, args.Src, args.Flat)
	if err != nil {
		return failedModule(err, operation.StageConfig)
	}
	result := tc.Executor.Pull(tc.Ctx, tc.Host, ops.TransferOptions{
		Direction: "pull", Src: args.Src, Dest: destination, Method: "auto",
		Overwrite: true, Backup: args.Backup,
		ValidateChecksum: true, DestinationExact: true,
		Check: tc.Check, Diff: tc.Diff, ConnectTimeout: tc.ConnectTimeout,
	})
	if result.Err != nil {
		return failedModule(result.Err, result.Stage)
	}
	if result.WouldChange || (tc.Check && result.Changed) {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true,
			Output: result.Output, Destination: result.Destination}
	}
	if result.Changed {
		return ModuleResult{Status: batch.StatusChanged, Changed: true,
			Output: result.Output, Destination: result.Destination}
	}
	return ModuleResult{Status: batch.StatusOK, Output: result.Output, Destination: result.Destination}
}

func pullDestination(projectRoot, baseDir, alias, dest, remote string, flat bool) (string, error) {
	absoluteRoot, err := resolveProjectPath(projectRoot, baseDir, dest)
	if err != nil {
		return "", fmt.Errorf("fetch dest 路径无效: %w", err)
	}
	remote = strings.TrimPrefix(remote, "/")
	name := filepath.Base(remote)
	var target string
	if flat {
		target = filepath.Join(absoluteRoot, name)
	} else {
		target = filepath.Join(absoluteRoot, alias, remote)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absoluteRoot, absoluteTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("fetch 目标路径逃逸: %s", target)
	}
	if err := rejectFetchSymlinks(absoluteRoot, absoluteTarget); err != nil {
		return "", err
	}
	return target, nil
}

func rejectFetchSymlinks(base, target string) error {
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("fetch 目标路径逃逸: %s", target)
	}
	current := base
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return nil
		}
		if statErr != nil {
			return fmt.Errorf("检查 fetch 目标失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fetch 目标路径包含符号链接，拒绝写入: %s", current)
		}
	}
	return nil
}

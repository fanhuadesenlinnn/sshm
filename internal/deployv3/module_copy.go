package deployv3

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
)

type copyArgs struct {
	Src      string `yaml:"src,omitempty"`
	Content  string `yaml:"content,omitempty"`
	Dest     string `yaml:"dest"`
	Backup   bool   `yaml:"backup,omitempty"`
	Checksum *bool  `yaml:"checksum,omitempty"`
	Mode     string `yaml:"mode,omitempty"`
}

type copyModule struct{}

func (m *copyModule) Name() string {
	return "copy"
}

func (m *copyModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args copyArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Dest) == "" {
		return nil, fmt.Errorf("copy 需要 dest")
	}
	if (args.Src == "") == (args.Content == "") {
		return nil, fmt.Errorf("copy 必须且只能包含 src 或 content")
	}
	return &args, nil
}

func (m *copyModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*copyArgs)
	return pushToRemote(tc, args.Src, args.Content, args.Dest, args.Backup, args.Checksum, args.Mode, tc.BaseDir)
}

// pushToRemote transfers either a local path or inline content and applies an
// optional remote mode. It is shared by copy and template.
func pushToRemote(tc TaskContext, src, content, dest string, backup bool, checksum *bool, mode, baseDir string) ModuleResult {
	localPath := ""
	tempPath := ""
	if content != "" || src == "" {
		var err error
		tempPath, err = writeTempContent(content)
		if err != nil {
			return failedModule(err, operation.StageConfig)
		}
		localPath = tempPath
		defer os.Remove(tempPath)
	} else {
		localPath = resolveRelative(baseDir, src)
		if _, err := os.Stat(localPath); err != nil {
			return failedModule(fmt.Errorf("本地文件不存在: %s", localPath), operation.StageConfig)
		}
	}
	validateChecksum := checksum == nil || *checksum
	result := tc.Executor.Push(tc.Ctx, tc.Host, ops.TransferOptions{
		Direction: "push", Src: localPath, Dest: dest, Method: "auto",
		Overwrite: true, Backup: backup, ValidateChecksum: validateChecksum,
		Check: tc.Check, Diff: tc.Diff, ConnectTimeout: tc.ConnectTimeout,
	})
	if result.Err != nil {
		return failedModule(result.Err, result.Stage)
	}
	if mode != "" && result.Changed {
		chmod := runRemote(tc, "chmod "+mode+" -- "+shellQuote(dest))
		if chmod.Status != batch.StatusOK {
			return chmod
		}
	}
	if result.WouldChange || (tc.Check && result.Changed) {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true,
			Output: result.Output, RC: 0}
	}
	if result.Changed {
		return ModuleResult{Status: batch.StatusChanged, Changed: true, Output: result.Output}
	}
	return ModuleResult{Status: batch.StatusOK, Output: result.Output}
}

func writeTempContent(content string) (string, error) {
	dir := os.TempDir()
	file, err := os.CreateTemp(dir, "sshm-content-*")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := safefile.Write(path, []byte(content), 0600); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

package deploy

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/safefile"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
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
	if args.Mode != "" {
		if _, err := strconv.ParseUint(args.Mode, 8, 12); err != nil {
			return nil, fmt.Errorf("copy mode 必须是八进制权限字符串")
		}
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

	changed := result.Changed
	wouldChange := result.WouldChange || (tc.Check && result.Changed)
	if mode != "" {
		info, statErr := statResult(tc, dest)
		if statErr != nil {
			return failedModule(statErr, operation.StageTransfer)
		}
		if !info.Exists || info.Mode.Perm() != os.FileMode(parseMode(mode)) {
			if tc.Check {
				changed = true
				wouldChange = true
			} else {
				chmod := runRemote(tc, "chmod "+mode+" -- "+shellquote.Single(dest))
				if chmod.Status != batch.StatusOK {
					return chmod
				}
				changed = true
				result.Output += "chmod " + mode + " " + dest + "\n"
			}
		}
	}
	if wouldChange {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true,
			Output: result.Output, RC: 0}
	}
	if changed {
		return ModuleResult{Status: batch.StatusChanged, Changed: true, Output: result.Output}
	}
	return ModuleResult{Status: batch.StatusOK, Output: result.Output}
}

func writeTempContent(content string) (string, error) {
	dir := os.TempDir()
	file, err := os.CreateTemp(dir, "sshmd-content-*")
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

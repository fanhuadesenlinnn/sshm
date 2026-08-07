package deployv3

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
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
	destination, err := pullDestination(tc.BaseDir, tc.Host.Alias, args.Dest, args.Src, args.Flat)
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

func pullDestination(root, alias, dest, remote string, flat bool) (string, error) {
	resolvedDest := resolveRelative(root, dest)
	absoluteRoot, err := filepath.Abs(resolvedDest)
	if err != nil {
		return "", err
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
	return target, nil
}

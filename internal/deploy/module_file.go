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
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
)

type fileArgs struct {
	Path  string `yaml:"path"`
	State string `yaml:"state"`
	Mode  string `yaml:"mode,omitempty"`
	Owner string `yaml:"owner,omitempty"`
	Group string `yaml:"group,omitempty"`
	Src   string `yaml:"src,omitempty"`
}

type fileModule struct{}

func (m *fileModule) Name() string {
	return "file"
}

func (m *fileModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args fileArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Path) == "" {
		return nil, fmt.Errorf("file 需要 path")
	}
	switch args.State {
	case "directory", "file", "link", "absent":
	default:
		return nil, fmt.Errorf("file state 必须是 directory、file、link 或 absent")
	}
	if args.State == "link" && args.Src == "" {
		return nil, fmt.Errorf("file state=link 需要 src")
	}
	if args.Mode != "" {
		if _, err := strconv.ParseUint(args.Mode, 8, 12); err != nil {
			return nil, fmt.Errorf("file mode 必须是八进制权限字符串")
		}
	}
	return &args, nil
}

func (m *fileModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*fileArgs)
	info, err := statResult(tc, args.Path)
	if err != nil {
		return failedModule(err, operation.StageTransfer)
	}
	switch args.State {
	case "absent":
		return m.absent(tc, args, info)
	case "directory":
		return m.directory(tc, args, info)
	case "file":
		return m.regularFile(tc, args, info)
	case "link":
		return m.link(tc, args, info)
	}
	return failedModule(fmt.Errorf("file state %q 未实现", args.State), operation.StageConfig)
}

func (m *fileModule) absent(tc TaskContext, args *fileArgs, info ops.RemoteFileInfo) ModuleResult {
	if !info.Exists {
		return ModuleResult{Status: batch.StatusOK}
	}
	if tc.Check {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "删除 " + args.Path + "\n"}
	}
	result := runRemote(tc, "rm -rf -- "+shellquote.Single(args.Path))
	if result.Status != batch.StatusOK {
		return result
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true, Output: "已删除 " + args.Path + "\n"}
}

func (m *fileModule) directory(tc TaskContext, args *fileArgs, info ops.RemoteFileInfo) ModuleResult {
	if info.Exists && !info.IsDir {
		return failedModule(fmt.Errorf("%s 已存在但不是目录", args.Path), operation.StageExecute)
	}
	changed := false
	if !info.Exists {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "创建目录 " + args.Path + "\n"}
		}
		mode := args.Mode
		if mode == "" {
			mode = "0755"
		}
		result := runRemote(tc, "mkdir -p -m "+mode+" -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	} else if args.Mode != "" && info.Mode.Perm() != os.FileMode(parseMode(args.Mode)) {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "修改权限 " + args.Path + "\n"}
		}
		result := runRemote(tc, "chmod "+args.Mode+" -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	}
	if args.Owner != "" || args.Group != "" {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "修改属主 " + args.Path + "\n"}
		}
		owner := ownerSpec(args.Owner, args.Group)
		result := runRemote(tc, "chown "+shellquote.Single(owner)+" -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	}
	return statusFor(changed, tc.Check)
}

func (m *fileModule) regularFile(tc TaskContext, args *fileArgs, info ops.RemoteFileInfo) ModuleResult {
	if info.Exists && info.IsDir {
		return failedModule(fmt.Errorf("%s 已存在但是目录", args.Path), operation.StageExecute)
	}
	changed := false
	if !info.Exists {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "创建文件 " + args.Path + "\n"}
		}
		result := runRemote(tc, "touch -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	}
	if args.Mode != "" && (!info.Exists || info.Mode.Perm() != os.FileMode(parseMode(args.Mode))) {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "修改权限 " + args.Path + "\n"}
		}
		result := runRemote(tc, "chmod "+args.Mode+" -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	}
	if args.Owner != "" || args.Group != "" {
		if tc.Check {
			return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "修改属主 " + args.Path + "\n"}
		}
		result := runRemote(tc, "chown "+shellquote.Single(ownerSpec(args.Owner, args.Group))+" -- "+shellquote.Single(args.Path))
		if result.Status != batch.StatusOK {
			return result
		}
		changed = true
	}
	return statusFor(changed, tc.Check)
}

func (m *fileModule) link(tc TaskContext, args *fileArgs, info ops.RemoteFileInfo) ModuleResult {
	if info.Exists && info.IsLink && info.Target == args.Src {
		return ModuleResult{Status: batch.StatusOK}
	}
	if tc.Check {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true, Output: "创建软链 " + args.Path + " -> " + args.Src + "\n"}
	}
	if info.Exists && !info.IsLink {
		if result := runRemote(tc, "rm -f -- "+shellquote.Single(args.Path)); result.Status != batch.StatusOK {
			return result
		}
	}
	result := runRemote(tc, "ln -sfn "+shellquote.Single(args.Src)+" "+shellquote.Single(args.Path))
	if result.Status != batch.StatusOK {
		return result
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true, Output: "创建软链 " + args.Path + "\n"}
}

func ownerSpec(owner, group string) string {
	switch {
	case owner != "" && group != "":
		return owner + ":" + group
	case group != "":
		return ":" + group
	default:
		return owner
	}
}

func parseMode(mode string) uint32 {
	value, _ := strconv.ParseUint(mode, 8, 12)
	return uint32(value)
}

func statusFor(changed, check bool) ModuleResult {
	if !changed {
		return ModuleResult{Status: batch.StatusOK}
	}
	if check {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true}
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true}
}

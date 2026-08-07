package deploy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/shellquote"
)

type commandArgs struct {
	Cmd     string `yaml:"cmd"`
	Chdir   string `yaml:"chdir,omitempty"`
	Creates string `yaml:"creates,omitempty"`
	Removes string `yaml:"removes,omitempty"`
}

// commandModule implements both command and shell; both run through the remote
// shell, keeping sshm's existing exec semantics.
type commandModule struct {
	name string
}

func (m commandModule) Name() string {
	return m.name
}

func (m commandModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args commandArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Cmd) == "" {
		return nil, fmt.Errorf("%s 需要 cmd", m.name)
	}
	if m.name == "command" {
		if marker := shellMeta(args.Cmd); marker != "" {
			return nil, fmt.Errorf("command 模块不经过 shell，cmd 不能包含 %q；如需管道/重定向/变量展开请改用 shell 模块", marker)
		}
	}
	if args.Creates != "" && args.Removes != "" {
		return nil, fmt.Errorf("creates 与 removes 不能同时使用")
	}
	return &args, nil
}

func (m commandModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*commandArgs)
	if tc.Check && !tc.CheckSafe {
		return ModuleResult{Status: batch.StatusSkipped, Output: "check 模式跳过不安全命令（设置 check_safe: true 以执行）\n"}
	}
	if args.Creates != "" {
		exists, err := remoteTest(tc, args.Creates)
		if err != nil {
			return failedModule(err, operation.StageExecute)
		}
		if exists {
			return ModuleResult{Status: batch.StatusOK, Output: fmt.Sprintf("creates 已存在: %s\n", args.Creates)}
		}
	}
	if args.Removes != "" {
		exists, err := remoteTest(tc, args.Removes)
		if err != nil {
			return failedModule(err, operation.StageExecute)
		}
		if !exists {
			return ModuleResult{Status: batch.StatusOK, Output: fmt.Sprintf("removes 不存在: %s\n", args.Removes)}
		}
	}
	command := args.Cmd
	if args.Chdir != "" {
		command = "cd " + shellquote.Single(args.Chdir) + " && " + command
	}
	return runRemote(tc, command)
}

var shellMetaChars = []string{";", "|", ">", "<", "&", "$", "`", "\n"}

// shellMeta returns the first shell metacharacter found in cmd, or "".
func shellMeta(cmd string) string {
	for _, marker := range shellMetaChars {
		if strings.Contains(cmd, marker) {
			return marker
		}
	}
	return ""
}

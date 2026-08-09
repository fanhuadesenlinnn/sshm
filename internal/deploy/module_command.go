package deploy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
)

type commandArgs struct {
	Cmd     string   `yaml:"cmd"`
	Argv    []string `yaml:"argv,omitempty"`
	Chdir   string   `yaml:"chdir,omitempty"`
	Creates string   `yaml:"creates,omitempty"`
	Removes string   `yaml:"removes,omitempty"`
}

// commandModule implements both command and shell. command is parsed into
// literal argv values and every value is quoted before it reaches the remote
// shell; shell intentionally keeps its full shell grammar.
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
	if m.name == "command" {
		hasCmd := strings.TrimSpace(args.Cmd) != ""
		hasArgv := args.Argv != nil
		if hasCmd == hasArgv {
			return nil, fmt.Errorf("command 必须且只能设置 cmd 或 argv")
		}
		argv := args.Argv
		if hasCmd {
			var err error
			argv, err = shellquote.Split(args.Cmd)
			if err != nil {
				return nil, fmt.Errorf("command cmd 解析失败: %w", err)
			}
		}
		if len(argv) == 0 || argv[0] == "" {
			return nil, fmt.Errorf("command 需要至少一个参数")
		}
		for _, value := range argv {
			if strings.ContainsRune(value, '\x00') {
				return nil, fmt.Errorf("command 参数不能包含 NUL")
			}
		}
		args.Cmd = shellquote.Command(argv)
	} else {
		if args.Argv != nil {
			return nil, fmt.Errorf("shell 只接受 cmd；结构化参数请使用 command.argv")
		}
		if strings.TrimSpace(args.Cmd) == "" {
			return nil, fmt.Errorf("shell 需要 cmd")
		}
		if strings.ContainsRune(args.Cmd, '\x00') {
			return nil, fmt.Errorf("shell cmd 不能包含 NUL")
		}
	}
	if args.Creates != "" && args.Removes != "" {
		return nil, fmt.Errorf("creates 与 removes 不能同时使用")
	}
	return &args, nil
}

// validateCommandTemplateBoundary keeps variable expansion from changing the
// number of command arguments. A literal cmd remains supported for concise
// static commands; templated values must be separate argv elements so each is
// rendered and quoted independently.
func validateCommandTemplateBoundary(moduleName string, node *yaml.Node) error {
	if moduleName != "command" || node == nil {
		return nil
	}
	mapping := node
	if mapping.Kind == yaml.DocumentNode && len(mapping.Content) == 1 {
		mapping = mapping.Content[0]
	}
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == "cmd" && strings.Contains(mapping.Content[index+1].Value, "{{") {
			return fmt.Errorf("command 的模板变量必须使用 argv 列表逐参数传递；cmd 仅支持不含模板的字面命令")
		}
	}
	return nil
}

func (m commandModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*commandArgs)
	if tc.Check && !tc.CheckSafe {
		return ModuleResult{
			Status:     batch.StatusSkipped,
			Output:     "check 模式跳过不安全命令（设置 check_safe: true 以执行）\n",
			SkipReason: "check 模式跳过（可设置 check_safe: true 执行）",
		}
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

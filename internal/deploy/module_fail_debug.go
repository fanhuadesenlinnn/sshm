package deploy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
)

type failArgs struct {
	Msg string `yaml:"msg,omitempty"`
}

type failModule struct{}

func (m *failModule) Name() string {
	return "fail"
}

func (m *failModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args failArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	return &args, nil
}

func (m *failModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*failArgs)
	message := args.Msg
	if message == "" {
		message = "fail 模块被触发"
	}
	return ModuleResult{Status: batch.StatusFailed, Err: fmt.Errorf("%s", message),
		Output: message + "\n", RC: 1, Stage: operation.StageExecute}
}

type debugArgs struct {
	Msg string `yaml:"msg,omitempty"`
	Var string `yaml:"var,omitempty"`
}

type debugModule struct{}

func (m *debugModule) Name() string {
	return "debug"
}

func (m *debugModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args debugArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	return &args, nil
}

func (m *debugModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*debugArgs)
	text := args.Msg
	if args.Var != "" {
		value, ok := debugLookup(taskEnv(tc), args.Var)
		if !ok {
			value = "<未定义>"
		}
		text = fmt.Sprintf("%s = %v", args.Var, value)
	}
	if text == "" {
		text = "debug"
	}
	if tc.Visible != nil {
		fmt.Fprintln(tc.Visible, "[debug] "+text)
	}
	return ModuleResult{Status: batch.StatusOK, Output: text + "\n"}
}

// debugLookup resolves a dotted path against the full runtime environment
// (vars, facts, registers and loop items).
func debugLookup(env map[string]any, path string) (any, bool) {
	var current any = any(env)
	for _, part := range strings.Split(path, ".") {
		value, ok := mapLookup(current, part)
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

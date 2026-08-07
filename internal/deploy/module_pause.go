package deploy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

type pauseArgs struct {
	Message string `yaml:"message,omitempty"`
}

type pauseModule struct{}

func (m *pauseModule) Name() string {
	return "pause"
}

func (m *pauseModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args pauseArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	return &args, nil
}

func (m *pauseModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*pauseArgs)
	message := strings.TrimSpace(args.Message)
	if message == "" {
		message = "确认继续?"
	}
	if !tc.PlayState.MarkPrompted(message) {
		return ModuleResult{Status: batch.StatusOK, Output: "已确认: " + message + "\n"}
	}
	if tc.Confirm == nil {
		return failedModule(fmt.Errorf("pause 步骤需要交互确认: %s", message), operation.StageConfig)
	}
	if err := tc.Confirm(message); err != nil {
		return failedModule(fmt.Errorf("用户拒绝 pause: %s（%v）", message, err), operation.StageConfig)
	}
	return ModuleResult{Status: batch.StatusOK, Output: "已确认: " + message + "\n"}
}

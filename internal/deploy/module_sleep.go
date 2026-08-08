package deploy

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
)

type sleepArgs struct {
	Seconds  int             `yaml:"seconds,omitempty"`
	Duration config.Duration `yaml:"duration,omitempty"`
}

type sleepModule struct{}

const maxSleepDuration = 24 * time.Hour

func (m *sleepModule) Name() string {
	return "sleep"
}

func (m *sleepModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args sleepArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if (args.Seconds == 0) == (args.Duration.Duration == 0) {
		return nil, fmt.Errorf("sleep 必须且只能包含 seconds 或 duration 之一")
	}
	if args.Seconds != 0 {
		maxSeconds := int(maxSleepDuration / time.Second)
		if args.Seconds < 1 || args.Seconds > maxSeconds {
			return nil, fmt.Errorf("sleep seconds 必须在 1 到 %d 之间（最多 24 小时）", maxSeconds)
		}
	}
	if args.Duration.Duration != 0 {
		if args.Duration.Duration <= 0 || args.Duration.Duration > maxSleepDuration {
			return nil, fmt.Errorf("sleep duration 必须大于 0 且不超过 24 小时")
		}
	}
	return &args, nil
}

func (m *sleepModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*sleepArgs)
	if tc.Check {
		return ModuleResult{
			Status:     batch.StatusSkipped,
			Output:     "check 模式跳过（sleep）\n",
			SkipReason: "check 模式跳过（sleep）",
		}
	}
	wait := time.Duration(args.Seconds) * time.Second
	if args.Duration.Duration > 0 {
		wait = args.Duration.Duration
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-tc.Ctx.Done():
		return failedModule(tc.Ctx.Err(), operation.StageTimeout)
	case <-timer.C:
		return ModuleResult{Status: batch.StatusOK, Output: fmt.Sprintf("已等待 %s\n", wait)}
	}
}

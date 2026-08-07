package deployv3

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

type serviceArgs struct {
	Name  string `yaml:"name"`
	State string `yaml:"state"`
}

type serviceModule struct{}

func (m *serviceModule) Name() string {
	return "service"
}

func (m *serviceModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args serviceArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Name) == "" {
		return nil, fmt.Errorf("service 需要 name")
	}
	switch args.State {
	case "started", "stopped", "restarted", "enabled", "disabled":
	default:
		return nil, fmt.Errorf("service state 必须是 started、stopped、restarted、enabled 或 disabled")
	}
	return &args, nil
}

func (m *serviceModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*serviceArgs)
	active, err := systemctlCheck(tc, args.Name, "is-active")
	if err != nil {
		return failedModule(err, operation.StageExecute)
	}
	enabled, err := systemctlCheck(tc, args.Name, "is-enabled")
	if err != nil {
		return failedModule(err, operation.StageExecute)
	}
	switch args.State {
	case "started":
		if active {
			return ModuleResult{Status: batch.StatusOK}
		}
		return systemctlAction(tc, args.Name, "start")
	case "stopped":
		if !active {
			return ModuleResult{Status: batch.StatusOK}
		}
		return systemctlAction(tc, args.Name, "stop")
	case "restarted":
		return systemctlAction(tc, args.Name, "restart")
	case "enabled":
		if enabled {
			return ModuleResult{Status: batch.StatusOK}
		}
		return systemctlAction(tc, args.Name, "enable")
	case "disabled":
		if !enabled {
			return ModuleResult{Status: batch.StatusOK}
		}
		return systemctlAction(tc, args.Name, "disable")
	}
	return failedModule(fmt.Errorf("service state %q 未实现", args.State), operation.StageConfig)
}

func systemctlCheck(tc TaskContext, name, verb string) (bool, error) {
	result, rc := execCommand(tc, "systemctl "+verb+" -- "+shellQuote(name))
	if result.Err != nil && !isRemoteRC(result.Err) {
		return false, result.Err
	}
	return rc == 0, nil
}

func systemctlAction(tc TaskContext, name, verb string) ModuleResult {
	if tc.Check {
		return ModuleResult{Status: batch.StatusWouldChange, Changed: true, WouldChange: true,
			Output: fmt.Sprintf("systemctl %s %s\n", verb, name)}
	}
	result := runRemote(tc, "systemctl "+verb+" -- "+shellQuote(name))
	if result.Status != batch.StatusOK {
		return result
	}
	return ModuleResult{Status: batch.StatusChanged, Changed: true,
		Output: fmt.Sprintf("systemctl %s %s\n", verb, name)}
}

func isRemoteRC(err error) bool {
	_, ok := remoteExitStatus(err)
	return ok
}

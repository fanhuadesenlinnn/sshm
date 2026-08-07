package deployv3

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

type waitForArgs struct {
	Path    string          `yaml:"path,omitempty"`
	Port    int             `yaml:"port,omitempty"`
	State   string          `yaml:"state,omitempty"`
	Timeout config.Duration `yaml:"timeout,omitempty"`
	Sleep   config.Duration `yaml:"sleep,omitempty"`
}

type waitForModule struct{}

func (m *waitForModule) Name() string {
	return "wait_for"
}

func (m *waitForModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args waitForArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if (args.Path == "") == (args.Port == 0) {
		return nil, fmt.Errorf("wait_for 必须且只能包含 path 或 port")
	}
	if args.State == "" {
		args.State = "present"
	}
	switch args.State {
	case "present", "absent", "started", "stopped":
	default:
		return nil, fmt.Errorf("wait_for state 必须是 present、absent、started 或 stopped")
	}
	if args.Port != 0 && (args.Port < 1 || args.Port > 65535) {
		return nil, fmt.Errorf("wait_for port 必须在 1 到 65535 之间")
	}
	return &args, nil
}

func (m *waitForModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*waitForArgs)
	timeout := args.Timeout.Duration
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	sleep := args.Sleep.Duration
	if sleep <= 0 {
		sleep = time.Second
	}
	deadline := time.Now().Add(timeout)
	wantPresent := args.State == "present" || args.State == "started"
	for {
		ok, err := m.check(tc, args)
		if err != nil {
			return failedModule(err, operation.StageExecute)
		}
		if ok == wantPresent {
			return ModuleResult{Status: batch.StatusOK}
		}
		if time.Now().After(deadline) || tc.Ctx.Err() != nil {
			target := "路径 " + args.Path
			if args.Port != 0 {
				target = "端口 " + strconv.Itoa(args.Port)
			}
			return failedModule(fmt.Errorf("等待 %s 超时（%s）", target, timeout), operation.StageTimeout)
		}
		select {
		case <-tc.Ctx.Done():
			return failedModule(tc.Ctx.Err(), operation.StageTimeout)
		case <-time.After(sleep):
		}
	}
}

func (m *waitForModule) check(tc TaskContext, args *waitForArgs) (bool, error) {
	if args.Path != "" {
		return remoteTest(tc, args.Path)
	}
	address := net.JoinHostPort(tc.Host.Host, strconv.Itoa(args.Port))
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}

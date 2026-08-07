package deployv3

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ops"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func remoteExitStatus(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var status interface{ ExitStatus() int }
	if errors.As(err, &status) {
		return status.ExitStatus(), true
	}
	return 1, false
}

// runRemote executes a command on the target host applying become and env.
func runRemote(tc TaskContext, command string) ModuleResult {
	if len(tc.Env) > 0 {
		parts := make([]string, 0, len(tc.Env))
		for key, value := range tc.Env {
			parts = append(parts, key+"="+shellQuote(value))
		}
		command = strings.Join(parts, " ") + " " + command
	}
	if tc.Become {
		user := tc.BecomeUser
		if user == "" {
			user = "root"
		}
		command = "sudo -n -u " + shellQuote(user) + " -- sh -c " + shellQuote(command)
	}
	result := tc.Executor.Exec(tc.Ctx, tc.Host, ops.ExecOptions{
		Command: command, ConnectTimeout: tc.ConnectTimeout,
		Stdout: tc.Visible, Stderr: tc.Visible,
	})
	rc, hasRC := remoteExitStatus(result.Err)
	if result.Err != nil {
		failed := failedModule(result.Err, result.Stage)
		if detail := firstUsefulLine(result.Output); detail != "" && !strings.Contains(failed.Err.Error(), detail) {
			failed.Err = fmt.Errorf("%s: %s", failed.Err, detail)
		}
		failed.Output = result.Output
		if hasRC {
			failed.RC = rc
		}
		return failed
	}
	return ModuleResult{Status: batch.StatusOK, Output: result.Output, RC: rc}
}

func firstUsefulLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return truncate(line, 200)
		}
	}
	return ""
}

func truncate(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

// remoteTest checks whether a path exists on the target host.
func remoteTest(tc TaskContext, path string) (bool, error) {
	result := tc.Executor.Exec(tc.Ctx, tc.Host, ops.ExecOptions{
		Command: "test -e " + shellQuote(path), ConnectTimeout: tc.ConnectTimeout,
	})
	if result.Err == nil {
		return true, nil
	}
	if _, hasRC := remoteExitStatus(result.Err); hasRC {
		return false, nil
	}
	return false, result.Err
}

func statResult(tc TaskContext, path string) (ops.RemoteFileInfo, error) {
	info, err := tc.Executor.Stat(tc.Ctx, tc.Host, path, tc.ConnectTimeout)
	if err != nil {
		return ops.RemoteFileInfo{}, operation.Wrap(operation.StageTransfer, err)
	}
	return info, nil
}

func execCommand(tc TaskContext, command string) (ops.Result, int) {
	result := tc.Executor.Exec(tc.Ctx, tc.Host, ops.ExecOptions{
		Command: command, ConnectTimeout: tc.ConnectTimeout,
	})
	rc, _ := remoteExitStatus(result.Err)
	return result, rc
}

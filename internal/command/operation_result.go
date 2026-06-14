package command

import (
	"fmt"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

func newOperationResult(host config.Host, output string, err error, fallback operation.FailureStage, retry string, duration time.Duration) operation.Result {
	stage := operation.StageOf(err, fallback)
	return operation.Result{
		Host: host, Output: output, Err: err, Stage: stage,
		Suggestion: operation.Suggestion(stage), RetryCommand: retry, Duration: duration,
	}
}

func printOperationFailure(result operation.Result) {
	ui.PrintError("%s (%s@%s:%d) 失败", result.Host.Alias, result.Host.User, result.Host.Host, result.Host.Port)
	fmt.Printf("  阶段: %s\n  原因: %v\n  建议: %s\n  重试: %s\n",
		result.Stage, result.Err, result.Suggestion, result.RetryCommand)
}

func writeOperationLog(action, detail string, results []operation.Result) error {
	logPath, err := operation.WriteLog(action, detail, results)
	if err != nil {
		return fmt.Errorf("写入操作日志失败: %w", err)
	}
	fmt.Printf("操作日志: %s\n", logPath)
	return nil
}

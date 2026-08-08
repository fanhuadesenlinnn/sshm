package command

import (
	"fmt"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func newOperationResult(host config.Host, output string, err error, fallback operation.FailureStage, retry string, duration time.Duration) operation.Result {
	stage := operation.StageOf(err, fallback)
	status := "success"
	if err != nil {
		status = "failed"
	}
	retry = nextCommandForFailure(host, stage, retry)
	return operation.Result{
		Host: host, Status: status, Output: output, Err: err, Stage: stage,
		Suggestion: operation.Suggestion(stage), RetryCommand: retry, Duration: duration,
	}
}

func nextCommandForFailure(host config.Host, stage operation.FailureStage, retry string) string {
	alias := host.Alias
	if alias == "" {
		alias = host.ID
	}
	switch stage {
	case operation.StageCredential:
		if name, ok := config.ManagedKeyName(host.Identity); ok {
			return fmt.Sprintf("sshmd key setup %s %s --yes", name, alias)
		}
		if host.Auth == "key" {
			return fmt.Sprintf("sshmd key setup <密钥名> %s --yes", alias)
		}
		return fmt.Sprintf("sshmd passwd %s", alias)
	case operation.StageVault:
		return "在交互终端重试以解锁 vault，或运行 sshmd doctor 检查凭据环境"
	default:
		return retry
	}
}

func skippedOperationResult(host config.Host, reason string) operation.Result {
	return operation.Result{
		Host: host, Status: "skipped", Output: "skipped: " + reason + "\n",
		Stage: operation.StageConfig,
	}
}

func printOperationFailure(result operation.Result) {
	ui.PrintError("%s (%s@%s:%d) 失败", result.Host.Alias, result.Host.User, result.Host.Host, result.Host.Port)
	fmt.Printf("  阶段: %s\n  原因: %v\n  建议: %s\n",
		result.Stage, result.Err, result.Suggestion)
	if result.RetryCommand != "" {
		label := "重试"
		if result.Stage == operation.StageCredential || result.Stage == operation.StageVault {
			label = "下一步"
		}
		fmt.Printf("  %s: %s\n", label, result.RetryCommand)
	}
}

func writeOperationLog(action, detail string, results []operation.Result) error {
	retention := 30 * 24 * time.Hour
	doc, err := config.NewRepository().Load()
	if err == nil {
		if !doc.Defaults.Logs.Enabled {
			return nil
		}
		retention = doc.Defaults.Logs.Retention.Duration
	}
	logPath, err := operation.WriteLogWithRetention(action, detail, results, retention)
	if err != nil {
		return fmt.Errorf("写入操作日志失败: %w", err)
	}
	fmt.Printf("操作日志: %s\n", logPath)
	return nil
}

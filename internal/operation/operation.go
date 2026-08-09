package operation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/safefile"
)

type FailureStage string

const (
	StageResolve    FailureStage = "resolve"
	StageNetwork    FailureStage = "network"
	StageJump       FailureStage = "jump-host"
	StageTrust      FailureStage = "host-trust"
	StageCredential FailureStage = "credential"
	StageAuth       FailureStage = "authentication"
	StageSession    FailureStage = "session"
	StageExecute    FailureStage = "execute"
	StageTransfer   FailureStage = "transfer"
	StageTimeout    FailureStage = "timeout"
	StageConfig     FailureStage = "config"
	StageConfirm    FailureStage = "confirmation"
	StageVault      FailureStage = "vault"
	StageUnknown    FailureStage = "unknown"
)

type StageError struct {
	Stage FailureStage
	Err   error
}

func (e *StageError) Error() string { return e.Err.Error() }
func (e *StageError) Unwrap() error { return e.Err }

func Wrap(stage FailureStage, err error) error {
	if err == nil {
		return nil
	}
	return &StageError{Stage: stage, Err: err}
}

func StageOf(err error, fallback FailureStage) FailureStage {
	if err == nil {
		return fallback
	}
	var staged *StageError
	if errors.As(err, &staged) {
		return staged.Stage
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "no such host"), strings.Contains(message, "无法解析"), strings.Contains(message, "解析主机"):
		return StageResolve
	case strings.Contains(message, "跳板机"), strings.Contains(message, "jump"):
		return StageJump
	case strings.Contains(message, "主机密钥"), strings.Contains(message, "无法验证主机"), strings.Contains(message, "host key"), strings.Contains(message, "knownhosts"):
		return StageTrust
	case strings.Contains(message, "未配置可用的认证凭据"), strings.Contains(message, "未配置托管密钥"):
		return StageCredential
	case strings.Contains(message, "vault"), strings.Contains(message, "密码库"), strings.Contains(message, "需要先解锁"):
		return StageVault
	case strings.Contains(message, "认证"), strings.Contains(message, "凭据"), strings.Contains(message, "authenticate"), strings.Contains(message, "permission denied"):
		return StageAuth
	case strings.Contains(message, "网络连接"), strings.Contains(message, "connection refused"), strings.Contains(message, "i/o timeout"), strings.Contains(message, "no route to host"):
		return StageNetwork
	case strings.Contains(message, "deadline exceeded"), strings.Contains(message, "context canceled"), strings.Contains(message, "超时或取消"):
		return StageTimeout
	case strings.Contains(message, "创建会话"), strings.Contains(message, "启动 shell"), strings.Contains(message, "pty"):
		return StageSession
	case strings.Contains(message, "sftp"), strings.Contains(message, "rsync"), strings.Contains(message, "远程目标"), strings.Contains(message, "本地目标"):
		return StageTransfer
	default:
		return fallback
	}
}

// IsConnectionFailure reports whether a stage means sshmd could not establish
// an authenticated SSH connection before the requested remote action ran.
func IsConnectionFailure(stage FailureStage) bool {
	switch stage {
	case StageResolve, StageNetwork, StageJump, StageTrust, StageCredential, StageAuth:
		return true
	default:
		return false
	}
}

func Suggestion(stage FailureStage) string {
	switch stage {
	case StageResolve:
		return "检查主机名或使用 sshmd edit 修正地址"
	case StageNetwork:
		return "检查地址、端口、防火墙和 SSH 服务"
	case StageJump:
		return "检查跳板机配置、认证和目标可达性"
	case StageTrust:
		return "核对主机密钥；确认无风险后再更新信任配置"
	case StageCredential:
		return "该主机尚未配置凭据；使用 sshmd passwd 或 sshmd key setup 后重试（此错误未尝试连接，若持续失败请另行确认主机网络可达）"
	case StageAuth:
		return "使用 sshmd passwd 或 sshmd key setup 配置可用凭据"
	case StageSession:
		return "检查远端 SSH 会话与 Shell 配置"
	case StageTransfer:
		return "检查源、目标、权限和 --overwrite/--backup 设置"
	case StageTimeout:
		return "检查超时设置、网络状态和远端命令耗时"
	case StageConfig:
		return "检查 deploy 配置并先运行 sshmd deploy validate"
	case StageConfirm:
		return "操作未获确认；核对目标和影响后再重试"
	case StageVault:
		return "解锁 sshmd 密码库并检查凭据"
	default:
		return "检查远程命令、权限和连接状态"
	}
}

type Result struct {
	Host         config.Host
	Status       string
	Output       string
	Err          error
	Stage        FailureStage
	Suggestion   string
	RetryCommand string
	Duration     time.Duration
}

func WriteLog(action, detail string, results []Result) (string, error) {
	return WriteLogWithRetention(action, detail, results, 30*24*time.Hour)
}

func WriteLogWithRetention(action, detail string, results []Result, retention time.Duration) (string, error) {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	if err := CleanExpired(retention); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405.000000000") + "-" + action
	dir := filepath.Join(config.LogsDir(), name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "action: %s\ndetail: %s\ncreated_at: %s\n\n", action, detail, time.Now().Format(time.RFC3339))
	for _, result := range results {
		status := result.Status
		if status == "" {
			status = "success"
		}
		if result.Err != nil && result.Status == "" {
			status = "failed"
		}
		fmt.Fprintf(&summary, "%s (%s) %s stage=%s duration=%s\n", result.Host.Alias, result.Host.Host, status, result.Stage, result.Duration)
		var body strings.Builder
		fmt.Fprintf(&body, "host: %s\naddress: %s@%s:%d\nstatus: %s\nstage: %s\nduration: %s\n",
			result.Host.Alias, result.Host.User, result.Host.Host, result.Host.Port, status, result.Stage, result.Duration)
		if result.Err != nil {
			fmt.Fprintf(&body, "error: %v\nsuggestion: %s\nretry: %s\n", result.Err, result.Suggestion, result.RetryCommand)
		}
		body.WriteString("\noutput:\n")
		body.WriteString(result.Output)
		filename := sanitize(result.Host.Alias) + "-" + sanitize(result.Host.Host) + ".log"
		if err := safefile.Write(filepath.Join(dir, filename), []byte(body.String()), 0600); err != nil {
			return "", err
		}
	}
	if err := safefile.Write(filepath.Join(dir, "summary.txt"), []byte(summary.String()), 0600); err != nil {
		return "", err
	}
	return dir, nil
}

func CleanExpired(retention time.Duration) error {
	entries, err := os.ReadDir(config.LogsDir())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-retention)
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(config.LogsDir(), entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitize(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

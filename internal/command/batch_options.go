package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/secret"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("命令退出，状态码 %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

type batchCLIOptions struct {
	Parallel       int
	Serial         int
	Timeout        time.Duration
	ConnectTimeout time.Duration
	FailFast       bool
	MaxFail        int
	MaxFailPercent int
	Yes            bool
	NoLog          bool
	Quiet          bool
	Exclude        []string
	ExcludeTags    []string
}

func parseBatchCLIOptions(args []string) (batchCLIOptions, []string, error) {
	var options batchCLIOptions
	var positionals []string
	value := func(index *int, flag string) (string, error) {
		if *index+1 >= len(args) {
			return "", fmt.Errorf("%s 缺少值", flag)
		}
		*index = *index + 1
		return args[*index], nil
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		switch args[i] {
		case "--parallel":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.Parallel, err = strconv.Atoi(raw)
			if err != nil || options.Parallel < 1 || options.Parallel > 128 {
				return options, nil, fmt.Errorf("--parallel 必须在 1 到 128 之间")
			}
		case "--serial":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.Serial, err = strconv.Atoi(raw)
			if err != nil || options.Serial < 1 {
				return options, nil, fmt.Errorf("--serial 必须是大于 0 的整数")
			}
		case "--timeout":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.Timeout, err = config.ParseHumanDuration(raw)
			if err != nil {
				return options, nil, fmt.Errorf("--timeout: %w", err)
			}
		case "--connect-timeout":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.ConnectTimeout, err = config.ParseHumanDuration(raw)
			if err != nil {
				return options, nil, fmt.Errorf("--connect-timeout: %w", err)
			}
		case "--max-fail":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.MaxFail, err = strconv.Atoi(raw)
			if err != nil || options.MaxFail < 1 {
				return options, nil, fmt.Errorf("--max-fail 必须是大于 0 的整数")
			}
		case "--max-fail-percent":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.MaxFailPercent, err = strconv.Atoi(raw)
			if err != nil || options.MaxFailPercent < 1 || options.MaxFailPercent > 100 {
				return options, nil, fmt.Errorf("--max-fail-percent 必须在 1 到 100 之间")
			}
		case "--fail-fast":
			options.FailFast = true
		case "--yes":
			options.Yes = true
		case "--no-log":
			options.NoLog = true
		case "--quiet":
			options.Quiet = true
		case "--exclude":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.Exclude = append(options.Exclude, raw)
		case "--exclude-tag":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, nil, err
			}
			options.ExcludeTags = append(options.ExcludeTags, raw)
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return options, nil, fmt.Errorf("未知批量执行选项: %s", args[i])
			}
			positionals = append(positionals, args[i])
		}
	}
	return options, positionals, nil
}

func (app *App) resolveBatchOptions(options batchCLIOptions, transfer bool) (batch.Options, time.Duration, time.Duration, config.LogDefaults, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return batch.Options{}, 0, 0, config.LogDefaults{}, err
	}
	if options.Parallel == 0 {
		options.Parallel = doc.Defaults.Batch.Parallel
	}
	if options.ConnectTimeout == 0 {
		options.ConnectTimeout = doc.Defaults.Batch.ConnectTimeout.Duration
	}
	if options.Timeout == 0 {
		options.Timeout = doc.Defaults.Exec.Timeout.Duration
		if transfer {
			options.Timeout = doc.Defaults.Transfer.Timeout.Duration
		}
	}
	batchOptions := batch.Options{
		Parallel:       options.Parallel,
		Serial:         options.Serial,
		FailFast:       options.FailFast,
		MaxFail:        options.MaxFail,
		MaxFailPercent: options.MaxFailPercent,
	}
	if err := batchOptions.Validate(); err != nil {
		return batch.Options{}, 0, 0, config.LogDefaults{}, err
	}
	return batchOptions, options.Timeout, options.ConnectTimeout, doc.Defaults.Logs, nil
}

func (app *App) unlockVaultForHosts(hosts []config.Host) error {
	needed := false
	for _, host := range hosts {
		if host.PasswordRef != "" {
			needed = true
			break
		}
		if _, managed := config.ManagedKeyName(host.Identity); managed {
			needed = true
			break
		}
	}
	if !needed || app.secretStore != nil {
		return nil
	}
	exists, err := secret.VaultExists(app.ConfigPath)
	if err != nil {
		return fmt.Errorf("检查 vault 失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("目标主机需要 vault 凭据，但 vault 尚未初始化")
	}
	if _, ok, err := app.secretStoreFromEnv(); ok || err != nil {
		return err
	}
	if !ui.IsTerminal() {
		return fmt.Errorf("目标主机需要 vault 凭据；非交互环境请设置 %s", masterPasswordEnv)
	}
	_, err = app.requireSecretStore()
	return err
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func batchStatus(result ops.Result) batch.Status {
	if result.Err == nil {
		return batch.StatusOK
	}
	if operation.IsConnectionFailure(result.Stage) {
		return batch.StatusUnreachable
	}
	return batch.StatusFailed
}

func exitErrorForBatch(result batch.RunResult) error {
	code := batch.ExitCode(result)
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code, Err: fmt.Errorf(
		"批量执行未完全成功：failed=%d unreachable=%d skipped=%d",
		result.Summary.Failed, result.Summary.Unreachable, result.Summary.Skipped,
	)}
}

// ExitCodeForError maps command failures to the documented process exit codes.
func ExitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if operation.IsConnectionFailure(operation.StageOf(err, operation.StageConfig)) {
		return 2
	}
	switch operation.StageOf(err, operation.StageConfig) {
	case operation.StageSession, operation.StageExecute, operation.StageTransfer, operation.StageTimeout:
		return 1
	default:
		return 3
	}
}

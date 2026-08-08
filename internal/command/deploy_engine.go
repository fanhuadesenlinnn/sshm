package command

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) loadDeploy(files []string) (*deploy.Catalog, []config.Host, error) {
	paths, err := deploy.Discover(files)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := deploy.Load(paths)
	if err != nil {
		return nil, nil, err
	}
	hf, err := app.Store.Load()
	if err != nil {
		return nil, nil, err
	}
	return catalog, hf.Hosts, nil
}

func (app *App) cmdDeployValidate(options deployCLIOptions) error {
	catalog, _, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := deploy.ValidateCatalog(catalog); err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, map[string]any{
			"ok": true, "sources": catalog.Sources, "plays": len(catalog.Plays),
		})
	}
	fmt.Printf("deploy v3 配置有效：%d 个文件，%d 个 play\n", len(catalog.Sources), len(catalog.Plays))
	return nil
}

func (app *App) cmdDeployList(options deployCLIOptions) error {
	catalog, _, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, catalog.Plays)
	}
	fmt.Printf("%-28s %-10s %s\n", "NAME", "STRATEGY", "SOURCE")
	sorted := append([]deploy.Play(nil), catalog.Plays...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, play := range sorted {
		fmt.Printf("%-28s %-10s %s\n", play.Name, play.StrategyOrDefault(), play.Source)
	}
	return nil
}

func (app *App) deployPlanCommand(options deployCLIOptions, show bool) error {
	if len(options.positionals) != 1 {
		command := "plan"
		if show {
			command = "show"
		}
		return fmt.Errorf("用法: sshm deploy %s <play> [-f 文件...] [目标覆盖]", command)
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if _, ok := catalog.ByName[options.positionals[0]]; !ok {
		return fmt.Errorf("未找到 deploy play %q；使用 sshm deploy list 查看全部 play", options.positionals[0])
	}
	overrides, _, err := app.deployOverrides(options)
	if err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, overrides)
	if err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WritePlanJSON(os.Stdout, plan)
	}
	deploy.WritePlanText(os.Stdout, plan)
	return nil
}

func (app *App) cmdDeployRun(options deployCLIOptions) error {
	if len(options.positionals) != 1 {
		return fmt.Errorf("用法: sshm deploy run <play> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if _, ok := catalog.ByName[options.positionals[0]]; !ok {
		candidates := catalog.PlayNames()
		if best, suggested := closestString(options.positionals[0], candidates, 3); suggested {
			return fmt.Errorf("未找到 deploy play %q；你是否想使用 %q？", options.positionals[0], best)
		}
		return fmt.Errorf("未找到 deploy play %q；使用 sshm deploy list 查看全部 play", options.positionals[0])
	}
	overrides, logs, err := app.deployOverrides(options)
	if err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, overrides)
	if err != nil {
		return err
	}
	if options.output == "json" && !options.batch.Yes {
		return fmt.Errorf("JSON 模式不会进行顶层交互确认；执行请显式使用 --yes")
	}
	if !options.batch.Yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("deploy 执行需要确认；非交互环境请使用 --yes")
		}
		deploy.WritePlanText(os.Stdout, plan)
		fmt.Println()
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}
	if err := app.unlockVaultForHosts(plan.Hosts); err != nil {
		return &ExitError{Code: 4, Err: err}
	}
	ctx, stop := signalContext()
	defer stop()
	executor := app.operationExecutor()
	defer executor.CloseSessions()
	runner := deploy.Runner{
		Executor: executor,
		Confirm: func(message string) error {
			if !ui.IsTerminal() {
				return fmt.Errorf("deploy pause/confirm 步骤需要交互终端: %s", message)
			}
			if !ui.ReadYesNo(message + " [y/N]: ") {
				return fmt.Errorf("用户拒绝 deploy pause/confirm: %s", message)
			}
			return nil
		},
		BecomePassword: func(host config.Host) (string, bool) {
			if envPassword := os.Getenv("SSHM_BECOME_PASSWORD"); envPassword != "" {
				return envPassword, true
			}
			if host.PasswordRef == "" {
				return "", false
			}
			if fs := app.tryGetSecretStore(); fs != nil {
				if password, err := fs.GetPassword(host.PasswordRef); err == nil {
					return password, true
				}
			}
			return "", false
		},
	}
	ndjson := options.output == "ndjson"
	progressEnabled := !ndjson && options.output != "json" && !options.batch.Quiet
	if !progressEnabled {
		runner.Visible = io.Discard
	} else {
		runner.Visible = os.Stdout
	}
	if ndjson {
		runner.Event = func(event deploy.PlayEvent) {
			_ = deploy.WriteNDJSON(os.Stdout, event)
		}
	} else if progressEnabled {
		runner.Event = func(event deploy.PlayEvent) {
			if event.Type == deploy.EventTaskDone {
				ui.RefreshLine("[%s] %-20s %-12s %s", event.Task, event.Host, event.Status, event.Reason)
			}
		}
	}
	result := runner.Run(ctx, plan)
	if progressEnabled {
		ui.EndProgress()
	}
	if logs.Enabled && !options.batch.NoLog {
		if _, err := deploy.WriteLog(plan, &result, logs.Retention.Duration); err != nil {
			return fmt.Errorf("写入 deploy 日志失败: %w", err)
		}
	}
	if ndjson {
		if err := deploy.WriteNDJSON(os.Stdout, result); err != nil {
			return err
		}
	} else if options.output == "json" {
		if err := deploy.WriteJSON(os.Stdout, result); err != nil {
			return err
		}
	} else {
		deploy.WriteRunText(os.Stdout, result)
	}
	if code := result.ExitCode(); code != 0 {
		return &ExitError{Code: code, Err: fmt.Errorf(
			"deploy 执行未完全成功：failed=%d unreachable=%d skipped=%d%s",
			result.Summary.Failed, result.Summary.Unreachable, result.Summary.Skipped,
			deployFailureHint(result),
		)}
	}
	return nil
}

// deployFailureHint summarizes the first failing hosts and suggests a safe
// way to re-locate the problem.
func deployFailureHint(result deploy.RunResult) string {
	var hosts []string
	for _, host := range result.Hosts {
		if host.Status != batch.StatusFailed && host.Status != batch.StatusUnreachable {
			continue
		}
		task := host.FailedTask
		if task == "" {
			task = "-"
		}
		hosts = append(hosts, fmt.Sprintf("%s(%s)", host.HostAlias, task))
		if len(hosts) >= 3 {
			break
		}
	}
	hint := ""
	if len(hosts) > 0 {
		hint += "；失败主机: " + strings.Join(hosts, ", ")
	}
	if result.Check {
		hint += "；check 模式下失败说明目标不满足条件，请结合失败主机与任务信息修复"
	} else {
		hint += "；可在 --check 下复跑定位（加 --diff 查看差异）"
	}
	return hint
}

func (app *App) deployOverrides(options deployCLIOptions) (deploy.Overrides, config.LogDefaults, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return deploy.Overrides{}, config.LogDefaults{}, err
	}
	overrides := deploy.Overrides{
		Parallel: options.batch.Parallel, Serial: options.batch.Serial,
		FailFast: options.batch.FailFast, MaxFail: options.batch.MaxFail,
		MaxFailPercent: options.batch.MaxFailPercent,
		Check:          options.check, Diff: options.diff, ExtraVars: options.extraVars,
		DefaultParallel:       doc.Defaults.Batch.Parallel,
		DefaultTimeout:        config.Duration{Duration: options.batch.Timeout},
		DefaultConnectTimeout: config.Duration{Duration: options.batch.ConnectTimeout},
	}
	if overrides.DefaultTimeout.Duration == 0 {
		overrides.DefaultTimeout = doc.Defaults.Exec.Timeout
	}
	if overrides.DefaultConnectTimeout.Duration == 0 {
		overrides.DefaultConnectTimeout = doc.Defaults.Batch.ConnectTimeout
	}
	if options.hasSelector {
		overrides.Targets = &options.selector
	}
	return overrides, doc.Defaults.Logs, nil
}

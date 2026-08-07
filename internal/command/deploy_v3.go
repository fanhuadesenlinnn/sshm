package command

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deployv3"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

// deployFileVersion detects whether the discovered deploy files are v2 or v3.
func (app *App) deployFileVersion(files []string) (int, error) {
	paths, err := deployv3.Discover(files)
	if err != nil {
		return 0, err
	}
	version := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, fmt.Errorf("读取 deploy 配置 %s 失败: %w", path, err)
		}
		var header struct {
			Version int `yaml:"version"`
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&header); err != nil {
			return 0, fmt.Errorf("解析 deploy 配置 %s 失败: %w", path, err)
		}
		if header.Version == 0 {
			return 0, fmt.Errorf("%s 缺少必填字段 version", path)
		}
		if version == 0 {
			version = header.Version
		}
		if version != header.Version {
			return 0, fmt.Errorf("deploy 文件版本不一致：%d 与 %d 混用", version, header.Version)
		}
	}
	if version == 0 {
		return 0, fmt.Errorf("未找到 deploy 配置；使用 sshm deploy init 或通过 --file 指定文件")
	}
	return version, nil
}

func (app *App) loadDeployV3(files []string) (*deployv3.Catalog, []config.Host, error) {
	paths, err := deployv3.Discover(files)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := deployv3.Load(paths)
	if err != nil {
		return nil, nil, err
	}
	hf, err := app.Store.Load()
	if err != nil {
		return nil, nil, err
	}
	return catalog, hf.Hosts, nil
}

func (app *App) cmdDeployValidateV3(options deployCLIOptions) error {
	catalog, _, err := app.loadDeployV3(options.files)
	if err != nil {
		return err
	}
	if err := deployv3.ValidateCatalog(catalog); err != nil {
		return err
	}
	if options.output == "json" {
		return deployv3.WriteJSON(os.Stdout, map[string]any{
			"ok": true, "sources": catalog.Sources, "plays": len(catalog.Plays),
		})
	}
	fmt.Printf("deploy v3 配置有效：%d 个文件，%d 个 play\n", len(catalog.Sources), len(catalog.Plays))
	return nil
}

func (app *App) cmdDeployListV3(options deployCLIOptions) error {
	catalog, _, err := app.loadDeployV3(options.files)
	if err != nil {
		return err
	}
	if options.output == "json" {
		return deployv3.WriteJSON(os.Stdout, catalog.Plays)
	}
	fmt.Printf("%-28s %-10s %s\n", "NAME", "STRATEGY", "SOURCE")
	sorted := append([]deployv3.Play(nil), catalog.Plays...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, play := range sorted {
		fmt.Printf("%-28s %-10s %s\n", play.Name, play.StrategyOrDefault(), play.Source)
	}
	return nil
}

func (app *App) deployPlanCommandV3(options deployCLIOptions, show bool) error {
	if len(options.positionals) != 1 {
		command := "plan"
		if show {
			command = "show"
		}
		return fmt.Errorf("用法: sshm deploy %s <play> [-f 文件...] [目标覆盖]", command)
	}
	catalog, hosts, err := app.loadDeployV3(options.files)
	if err != nil {
		return err
	}
	if _, ok := catalog.ByName[options.positionals[0]]; !ok {
		return fmt.Errorf("未找到 deploy play %q；使用 sshm deploy list 查看全部 play", options.positionals[0])
	}
	overrides, _, err := app.deployV3Overrides(options)
	if err != nil {
		return err
	}
	plan, err := deployv3.BuildPlan(catalog, options.positionals[0], hosts, overrides)
	if err != nil {
		return err
	}
	if options.output == "json" {
		return deployv3.WritePlanJSON(os.Stdout, plan)
	}
	deployv3.WritePlanText(os.Stdout, plan)
	return nil
}

func (app *App) cmdDeployRunV3(options deployCLIOptions) error {
	if len(options.positionals) != 1 {
		return fmt.Errorf("用法: sshm deploy run <play> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]")
	}
	catalog, hosts, err := app.loadDeployV3(options.files)
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
	overrides, logs, err := app.deployV3Overrides(options)
	if err != nil {
		return err
	}
	plan, err := deployv3.BuildPlan(catalog, options.positionals[0], hosts, overrides)
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
		deployv3.WritePlanText(os.Stdout, plan)
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
	runner := deployv3.Runner{
		Executor: app.operationExecutor(),
		Confirm: func(message string) error {
			if !ui.IsTerminal() {
				return fmt.Errorf("deploy pause 步骤需要交互终端: %s", message)
			}
			if !ui.ReadYesNo(message + " [y/N]: ") {
				return fmt.Errorf("用户拒绝 deploy pause: %s", message)
			}
			return nil
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
		runner.Event = func(event deployv3.V3Event) {
			_ = deployv3.WriteNDJSON(os.Stdout, event)
		}
	} else if progressEnabled {
		runner.Event = func(event deployv3.V3Event) {
			if event.Type == deployv3.EventTaskDone {
				ui.RefreshLine("[%s] %-20s %-12s %s", event.Task, event.Host, event.Status, event.Reason)
			}
		}
	}
	result := runner.Run(ctx, plan)
	if progressEnabled {
		ui.EndProgress()
	}
	if logs.Enabled && !options.batch.NoLog {
		if _, err := deployv3.WriteLog(plan, &result, logs.Retention.Duration); err != nil {
			return fmt.Errorf("写入 deploy 日志失败: %w", err)
		}
	}
	if ndjson {
		if err := deployv3.WriteNDJSON(os.Stdout, result); err != nil {
			return err
		}
	} else if options.output == "json" {
		if err := deployv3.WriteJSON(os.Stdout, result); err != nil {
			return err
		}
	} else {
		deployv3.WriteRunText(os.Stdout, result)
	}
	if code := result.ExitCode(); code != 0 {
		return &ExitError{Code: code, Err: fmt.Errorf(
			"deploy 执行未完全成功：failed=%d unreachable=%d skipped=%d",
			result.Summary.Failed, result.Summary.Unreachable, result.Summary.Skipped,
		)}
	}
	return nil
}

func (app *App) deployV3Overrides(options deployCLIOptions) (deployv3.Overrides, config.LogDefaults, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return deployv3.Overrides{}, config.LogDefaults{}, err
	}
	overrides := deployv3.Overrides{
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

func (app *App) cmdDeployMigrate(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.hasSelector || hasDeployRunOptions(options) || options.overwrite || options.output != "text" {
		return fmt.Errorf("deploy migrate 仅支持 -f 和 --stdout")
	}
	if options.deployVersion != 0 {
		return fmt.Errorf("--version 仅适用于 deploy init")
	}
	paths, err := deploy.Discover(options.files, "")
	if err != nil {
		return err
	}
	migration, err := deployv3.MigrateFromV2(paths)
	if err != nil {
		return err
	}
	for _, warning := range migration.Warnings {
		ui.PrintWarn("%s", warning)
	}
	if options.stdout {
		_, err = os.Stdout.Write(migration.YAML)
		return err
	}
	path := config.DeployFilePath()
	if len(options.files) == 1 {
		path = strings.TrimSuffix(options.files[0], fileExt(options.files[0])) + ".v3.yaml"
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s 已存在；请先移走或使用 --stdout 查看", path)
	}
	if err := os.WriteFile(path, migration.YAML, 0600); err != nil {
		return err
	}
	fmt.Printf("已生成 v3 配置: %s（%d 个 play，%d 条警告）\n", path, migration.PlayCount, len(migration.Warnings))
	return nil
}

func fileExt(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 {
		return path[index:]
	}
	return ""
}

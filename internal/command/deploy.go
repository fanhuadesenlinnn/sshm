package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/safefile"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
	"github.com/spf13/cobra"
)

func newDeployCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:     "deploy",
		Short:   commandShort("deploy", "运行 Deploy v2 轻量编排"),
		GroupID: commandGroupID("deploy"),
		Long:    "运行 Deploy v2 轻量编排。deploy 从 ~/.sshm/deploy.yaml 和 ~/.sshm/deploy.d/*.yaml 读取 profile；显式 --file 时只读取指定文件。",
		Example: strings.TrimSpace(`
sshm deploy init
sshm deploy list
sshm deploy plan webapp --host web01
sshm deploy run webapp --tag prod --check --yes`),
		RunE: func(_ *cobra.Command, _ []string) error {
			app.printDeployHelp()
			return nil
		},
	}
	children := []struct {
		use, short, long, example string
		aliases                   []string
		run                       func([]string) error
	}{
		{
			use: "init [-f 文件] [--stdout] [--overwrite]", short: "生成中文 Deploy v2 示例",
			long: "生成 Deploy v2 示例配置。默认写入 ~/.sshm/deploy.yaml；使用 --stdout 可只打印不写文件。",
			example: strings.TrimSpace(`
sshm deploy init
sshm deploy init -f ./deploy.yaml
sshm deploy init --stdout`),
			run: app.cmdDeployInit,
		},
		{
			use: "validate [-f 文件...] [--output text|json]", short: "严格校验 Deploy v2 配置",
			long: "校验 Deploy v2 配置、profile、目标选择和 action DSL。适合在真正执行前做本地检查。",
			example: strings.TrimSpace(`
sshm deploy validate
sshm deploy validate -f ./deploy.yaml
sshm deploy validate -f base.yaml -f project.yaml --output json`),
			run: app.cmdDeployValidate,
		},
		{
			use: "list [-f 文件...] [--output text|json]", short: "列出 deploy profiles",
			long: "列出可用 Deploy profiles 及其来源文件。",
			example: strings.TrimSpace(`
sshm deploy list
sshm deploy list -f ./deploy.yaml
sshm deploy list --output json`),
			aliases: []string{"ls"}, run: app.cmdDeployList,
		},
		{
			use: "show <profile> [-f 文件...] [目标覆盖]", short: "展示解析后的 profile",
			long: "展示 profile 解析后的配置，包含目标、步骤和批量策略；不会连接远端。",
			example: strings.TrimSpace(`
sshm deploy show webapp
sshm deploy show webapp --host web01
sshm deploy show webapp -f ./deploy.yaml --tag prod`),
			run: func(args []string) error { return app.deployPlanCommand(args, true) },
		},
		{
			use: "plan <profile> [-f 文件...] [目标覆盖]", short: "静态展示执行计划，不连接远端",
			long: "生成 Deploy 执行计划并展示将要影响的主机与步骤；不会连接远端。",
			example: strings.TrimSpace(`
sshm deploy plan webapp
sshm deploy plan webapp --host web01
sshm deploy plan webapp --tag prod --output json`),
			run: func(args []string) error { return app.deployPlanCommand(args, false) },
		},
		{
			use: "run <profile> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]", short: "执行 profile，支持 --check 与 --diff",
			long: "执行 Deploy profile。默认会确认执行计划；脚本或 CI 中请显式传入 --yes。--check 只检查变化，--diff 展示差异。",
			example: strings.TrimSpace(`
sshm deploy run webapp --host web01 --check
sshm deploy run webapp --tag prod --serial 2 --max-fail 1 --yes
sshm deploy run webapp -f base.yaml -f project.yaml --all --diff --yes`),
			run: app.cmdDeployRun,
		},
	}
	for _, child := range children {
		child := child
		command.AddCommand(&cobra.Command{
			Use: child.use, Short: child.short, Long: child.long, Example: child.example,
			Aliases: child.aliases, DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error { return child.run(args) },
		})
	}
	return command
}

func (app *App) cmdDeploy(args []string) error {
	if len(args) == 0 {
		app.printDeployHelp()
		return nil
	}
	switch args[0] {
	case "init":
		return app.cmdDeployInit(args[1:])
	case "validate":
		return app.cmdDeployValidate(args[1:])
	case "list", "ls":
		return app.cmdDeployList(args[1:])
	case "show":
		return app.deployPlanCommand(args[1:], true)
	case "plan":
		return app.deployPlanCommand(args[1:], false)
	case "run":
		return app.cmdDeployRun(args[1:])
	case "help", "-h", "--help":
		app.printDeployHelp()
		return nil
	default:
		return fmt.Errorf("未知 deploy 命令 %q；使用 sshm deploy help 查看帮助", args[0])
	}
}

func (app *App) printDeployHelp() {
	ui.PrintHeader("Deploy v2 轻量编排")
	fmt.Println()
	fmt.Println("  deploy init [-f 文件] [--stdout] [--overwrite]")
	fmt.Println("  deploy validate [-f 文件...] [--output text|json]")
	fmt.Println("  deploy list [-f 文件...] [--output text|json]")
	fmt.Println("  deploy show <profile> [-f 文件...] [目标覆盖]")
	fmt.Println("  deploy plan <profile> [-f 文件...] [目标覆盖]")
	fmt.Println("  deploy run <profile> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]")
	fmt.Println()
	fmt.Println("  目标覆盖: --host 主机[,主机] | --tag 标签[,标签] | --all")
	fmt.Println("  批量选项: --serial N --parallel N --fail-fast --max-fail N --max-fail-percent N")
	fmt.Println("  未使用 -f 时加载 ~/.sshm/deploy.yaml 与按文件名排序的 ~/.sshm/deploy.d/*.yaml")
	fmt.Println("  当前目录的 sshm.deploy.yaml 不会被隐式加载")
	fmt.Println()
}

type deployCLIOptions struct {
	files       []string
	positionals []string
	output      string
	selector    deploy.TargetSelector
	hasSelector bool
	batch       batchCLIOptions
	stdout      bool
	overwrite   bool
	check       bool
	diff        bool
}

func parseDeployCLIOptions(args []string) (deployCLIOptions, error) {
	options := deployCLIOptions{output: "text"}
	value := func(index *int, flag string) (string, error) {
		if *index+1 >= len(args) {
			return "", fmt.Errorf("%s 缺少值", flag)
		}
		*index++
		return args[*index], nil
	}
	positiveInt := func(index *int, flag string, max int) (int, error) {
		raw, err := value(index, flag)
		if err != nil {
			return 0, err
		}
		number, err := strconv.Atoi(raw)
		if err != nil || number < 1 || (max > 0 && number > max) {
			if max > 0 {
				return 0, fmt.Errorf("%s 必须在 1 到 %d 之间", flag, max)
			}
			return 0, fmt.Errorf("%s 必须是大于 0 的整数", flag)
		}
		return number, nil
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--file":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.files = append(options.files, item)
		case "--output":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.output = item
		case "--host":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.selector.Hosts = append(options.selector.Hosts, item)
			options.hasSelector = true
		case "--tag":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.selector.Tags = append(options.selector.Tags, item)
			options.hasSelector = true
		case "--all":
			options.selector.All = true
			options.hasSelector = true
		case "--parallel":
			number, err := positiveInt(&i, args[i], 128)
			if err != nil {
				return options, err
			}
			options.batch.Parallel = number
		case "--serial":
			number, err := positiveInt(&i, args[i], 0)
			if err != nil {
				return options, err
			}
			options.batch.Serial = number
		case "--max-fail":
			number, err := positiveInt(&i, args[i], 0)
			if err != nil {
				return options, err
			}
			options.batch.MaxFail = number
		case "--max-fail-percent":
			number, err := positiveInt(&i, args[i], 100)
			if err != nil {
				return options, err
			}
			options.batch.MaxFailPercent = number
		case "--timeout", "--connect-timeout":
			raw, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			duration, err := config.ParseHumanDuration(raw)
			if err != nil {
				return options, fmt.Errorf("%s: %w", args[i-1], err)
			}
			if args[i-1] == "--timeout" {
				options.batch.Timeout = duration
			} else {
				options.batch.ConnectTimeout = duration
			}
		case "--fail-fast":
			options.batch.FailFast = true
		case "--yes":
			options.batch.Yes = true
		case "--no-log":
			options.batch.NoLog = true
		case "--quiet":
			options.batch.Quiet = true
		case "--check":
			options.check = true
		case "--diff":
			options.diff = true
		case "--stdout":
			options.stdout = true
		case "--overwrite":
			options.overwrite = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return options, fmt.Errorf("未知 deploy 选项: %s", args[i])
			}
			options.positionals = append(options.positionals, args[i])
		}
	}
	if options.output != "text" && options.output != "json" {
		return options, fmt.Errorf("--output 必须是 text 或 json")
	}
	if options.selector.All && (len(options.selector.Hosts) > 0 || len(options.selector.Tags) > 0) {
		return options, fmt.Errorf("--all 不能与 --host 或 --tag 混用")
	}
	return options, nil
}

func (app *App) cmdDeployInit(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.output != "text" || options.hasSelector || hasDeployRunOptions(options) {
		return fmt.Errorf("deploy init 仅支持 -f、--stdout 和 --overwrite")
	}
	if len(options.files) > 1 {
		return fmt.Errorf("deploy init 只能指定一个 -f")
	}
	if options.stdout {
		fmt.Print(deploy.Sample)
		return nil
	}
	path := config.DeployFilePath()
	if len(options.files) == 1 {
		path = config.ExpandPath(options.files[0])
	}
	if _, err := os.Stat(path); err == nil && !options.overwrite {
		return fmt.Errorf("%s 已存在；使用 --overwrite 明确覆盖", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := safefile.Write(path, []byte(deploy.Sample), 0600); err != nil {
		return err
	}
	fmt.Printf("已生成 deploy 配置: %s\n", path)
	fmt.Println()
	fmt.Println("下一步:")
	fileArg := ""
	if len(options.files) == 1 {
		fileArg = " -f " + shellSingleQuote(path)
	}
	fmt.Printf("  sshm deploy validate%s\n", fileArg)
	if !app.hasHostWithAllTags("prod") {
		fmt.Println("  sshm add web01 root@10.0.0.11 --tags prod")
	}
	fmt.Printf("  sshm deploy plan update-app%s\n", fileArg)
	return nil
}

func (app *App) hasHostWithAllTags(tags ...string) bool {
	hf, err := app.Store.Load()
	if err != nil {
		return false
	}
	for _, host := range hf.Hosts {
		if host.MatchTags(tags) {
			return true
		}
	}
	return false
}

func hasDeployRunOptions(options deployCLIOptions) bool {
	return options.batch.Parallel != 0 || options.batch.Serial != 0 || options.batch.Timeout != 0 ||
		options.batch.ConnectTimeout != 0 || options.batch.FailFast || options.batch.MaxFail != 0 ||
		options.batch.MaxFailPercent != 0 || options.batch.Yes || options.batch.NoLog || options.batch.Quiet ||
		options.check || options.diff
}

func (app *App) loadDeploy(files []string) (*deploy.Catalog, []config.Host, error) {
	paths, err := deploy.Discover(files, "")
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

func (app *App) cmdDeployValidate(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.hasSelector || hasDeployRunOptions(options) || options.stdout || options.overwrite {
		return fmt.Errorf("deploy validate 仅支持 -f 和 --output")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := deploy.ValidateCatalogAllowEmptyTargetMatches(catalog, hosts); err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, map[string]any{"ok": true, "sources": catalog.Sources, "profiles": len(catalog.Profiles), "handlers": len(catalog.Handlers)})
	}
	fmt.Printf("deploy 配置有效：%d 个文件，%d 个 profile，%d 个 handler\n", len(catalog.Sources), len(catalog.Profiles), len(catalog.Handlers))
	return nil
}

func (app *App) cmdDeployList(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.hasSelector || hasDeployRunOptions(options) || options.stdout || options.overwrite {
		return fmt.Errorf("deploy list 仅支持 -f 和 --output")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := deploy.ValidateCatalogAllowEmptyTargetMatches(catalog, hosts); err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, catalog.Profiles)
	}
	fmt.Printf("%-28s %-24s %-6s %-8s %-8s %s\n", "NAME", "TARGETS", "STEPS", "SERIAL", "PARALLEL", "SOURCE")
	for _, profile := range catalog.Profiles {
		fmt.Printf("%-28s %-24s %-6d %-8d %-8d %s\n", profile.Name, profile.Targets.String(), len(profile.Steps),
			profile.Serial, profile.Parallel, profile.Source)
	}
	return nil
}

func (app *App) deployPlanCommand(args []string, show bool) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 1 {
		command := "plan"
		if show {
			command = "show"
		}
		return fmt.Errorf("用法: sshm deploy %s <profile> [-f 文件...] [目标覆盖]", command)
	}
	if options.batch.Yes || options.batch.NoLog || options.batch.Quiet || options.stdout || options.overwrite {
		return fmt.Errorf("deploy plan/show 不支持当前选项")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := validateSelectedDeployProfile(catalog, options.positionals[0], hosts, options); err != nil {
		return err
	}
	overrides, _, err := app.deployOverrides(options)
	if err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, overrides)
	if err != nil {
		return err
	}
	return writeDeployPlan(plan, options.output)
}

func (app *App) cmdDeployRun(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 1 {
		return fmt.Errorf("用法: sshm deploy run <profile> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]")
	}
	if options.stdout || options.overwrite {
		return fmt.Errorf("deploy run 不支持 --stdout 或 --overwrite")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := validateSelectedDeployProfile(catalog, options.positionals[0], hosts, options); err != nil {
		return err
	}
	overrides, logs, err := app.deployOverrides(options)
	if err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, overrides)
	if err != nil {
		return err
	}
	return app.executeDeployPlan(plan, options, logs)
}

func (app *App) deployOverrides(options deployCLIOptions) (deploy.Overrides, config.LogDefaults, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return deploy.Overrides{}, config.LogDefaults{}, err
	}
	overrides := deploy.Overrides{
		Parallel: options.batch.Parallel, Serial: options.batch.Serial,
		FailFast: options.batch.FailFast, MaxFail: options.batch.MaxFail, MaxFailPercent: options.batch.MaxFailPercent,
		Check: options.check, Diff: options.diff, DefaultParallel: doc.Defaults.Batch.Parallel,
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

func validateSelectedDeployProfile(catalog *deploy.Catalog, name string, hosts []config.Host, options deployCLIOptions) error {
	profile, ok := catalog.ByName[name]
	if !ok {
		candidates := make([]string, 0, len(catalog.Profiles))
		for _, profile := range catalog.Profiles {
			candidates = append(candidates, profile.Name)
		}
		if best, suggested := closestString(name, candidates, 3); suggested {
			return fmt.Errorf("未找到 deploy profile %q；你是否想使用 %q？使用 sshm deploy list 查看全部 profile", name, best)
		}
		return fmt.Errorf("未找到 deploy profile %q；使用 sshm deploy list 查看全部 profile", name)
	}
	if options.hasSelector {
		profile.Targets = options.selector
	}
	if err := deploy.ValidateProfile(profile, hosts, false); err != nil {
		if len(hosts) == 0 {
			return fmt.Errorf("当前还没有主机；请先运行 sshm add web01 root@10.0.0.11 --tags prod，或修改 deploy targets: %w", err)
		}
		return err
	}
	for _, step := range profile.Steps {
		for _, handler := range step.Notify {
			if _, ok := catalog.HandlerByName[handler]; !ok {
				return fmt.Errorf("profile %q 引用了不存在的 handler %q", name, handler)
			}
		}
	}
	return nil
}

func writeDeployPlan(plan deploy.Plan, output string) error {
	if output == "json" {
		return deploy.WriteJSON(os.Stdout, plan.JSON())
	}
	deploy.WritePlanText(os.Stdout, plan)
	return nil
}

func (app *App) executeDeployPlan(plan deploy.Plan, options deployCLIOptions, logs config.LogDefaults) error {
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
	runner := deploy.Runner{
		Executor: app.operationExecutor(),
		Confirm: func(message string) error {
			if !ui.IsTerminal() {
				return fmt.Errorf("deploy confirm step 需要交互终端: %s", message)
			}
			if !ui.ReadYesNo(message + " [y/N]: ") {
				return fmt.Errorf("用户拒绝 deploy confirm: %s", message)
			}
			return nil
		},
	}
	progressEnabled := options.output != "json" && !options.batch.Quiet
	if progressEnabled {
		runner.Progress = func(done, total int, result deploy.HostResult) {
			ui.RefreshLine("[%d/%d] 已完成 %-20s %-12s", done, total, result.HostAlias, result.Status)
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
	if options.output == "json" {
		if err := deploy.WriteJSON(os.Stdout, result); err != nil {
			return err
		}
	} else {
		deploy.WriteRunText(os.Stdout, result)
	}
	if code := result.ExitCode(); code != 0 {
		return &ExitError{Code: code, Err: fmt.Errorf(
			"deploy 执行未完全成功：failed=%d unreachable=%d skipped=%d",
			result.Summary.Failed, result.Summary.Unreachable, result.Summary.Skipped,
		)}
	}
	return nil
}

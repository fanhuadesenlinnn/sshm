package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/safefile"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
	"github.com/spf13/cobra"
)

func newDeployCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:     "deploy",
		Short:   commandShort("deploy", "运行 Deploy v3 轻量编排"),
		GroupID: commandGroupID("deploy"),
		Long:    "运行 Deploy v3 轻量编排。deploy 从 ~/.sshmd/deploy.yaml 和 ~/.sshmd/deploy.d/*.yaml 读取 playbook；显式 --file 时只读取指定文件。",
		Example: strings.TrimSpace(`
sshmd deploy init
sshmd deploy list
sshmd deploy plan webapp --host web01
sshmd deploy run webapp --tag prod --check --yes`),
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
			use: "init [-f 文件 | --dir 目录] [--stdout] [--overwrite]", short: "生成 Deploy v3 示例",
			long: "生成 Deploy v3 示例配置。默认写入 ~/.sshmd/deploy.yaml；--dir 生成整套可校验的 demo 目录（含 templates/tasks/vars/README）；--stdout 只打印不写文件。",
			example: strings.TrimSpace(`
sshmd deploy init
sshmd deploy init -f ./deploy.yaml
sshmd deploy init --dir ./my-deploy
sshmd deploy init --stdout`),
			run: app.cmdDeployInit,
		},
		{
			use: "validate [-f 文件...] [--output text|json]", short: "严格校验 Deploy 配置",
			long: "校验 Deploy 配置、play、目标选择和模块参数。适合在真正执行前做本地检查。",
			example: strings.TrimSpace(`
sshmd deploy validate
sshmd deploy validate -f ./deploy.yaml
sshmd deploy validate -f base.yaml -f project.yaml --output json`),
			run: app.deployValidate,
		},
		{
			use: "list [-f 文件...] [--output text|json]", short: "列出 deploy plays",
			long: "列出可用 Deploy plays 及其来源文件。",
			example: strings.TrimSpace(`
sshmd deploy list
sshmd deploy list -f ./deploy.yaml
sshmd deploy list --output json`),
			aliases: []string{"ls"}, run: app.deployList,
		},
		{
			use: "show <play> [-f 文件...] [目标覆盖]", short: "展示解析后的 play",
			long: "展示 play 解析后的配置，包含目标、任务和批量策略；不会连接远端。",
			example: strings.TrimSpace(`
sshmd deploy show webapp
sshmd deploy show webapp --host web01
sshmd deploy show webapp -f ./deploy.yaml --tag prod`),
			run: func(args []string) error { return app.deployPlan(args, true) },
		},
		{
			use: "plan <play> [-f 文件...] [目标覆盖]", short: "静态展示执行计划，不连接远端",
			long: "生成 Deploy 执行计划并展示将要影响的主机与任务；不会连接远端。",
			example: strings.TrimSpace(`
sshmd deploy plan webapp
sshmd deploy plan webapp --host web01
sshmd deploy plan webapp --tag prod --output json`),
			run: func(args []string) error { return app.deployPlan(args, false) },
		},
		{
			use: "run <play> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]", short: "执行 play，支持 --check 与 --diff",
			long: "执行 Deploy play。默认会确认执行计划；脚本或 CI 中请显式传入 --yes。--check 只检查变化，--diff 展示差异。",
			example: strings.TrimSpace(`
sshmd deploy run webapp --host web01 --check
sshmd deploy run webapp --tag prod --serial 2 --max-fail 1 --yes
sshmd deploy run webapp -f base.yaml -f project.yaml --all --diff --yes`),
			run: app.deployRun,
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
		return app.deployValidate(args[1:])
	case "list", "ls":
		return app.deployList(args[1:])
	case "show":
		return app.deployPlan(args[1:], true)
	case "plan":
		return app.deployPlan(args[1:], false)
	case "run":
		return app.deployRun(args[1:])
	case "help", "-h", "--help":
		app.printDeployHelp()
		return nil
	default:
		return fmt.Errorf("未知 deploy 命令 %q；使用 sshmd deploy help 查看帮助", args[0])
	}
}

func (app *App) deployValidate(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	return app.cmdDeployValidate(options)
}

func (app *App) deployList(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	return app.cmdDeployList(options)
}

func (app *App) deployPlan(args []string, show bool) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	return app.deployPlanCommand(options, show)
}

func (app *App) deployRun(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	return app.cmdDeployRun(options)
}

func (app *App) printDeployHelp() {
	ui.PrintHeader("Deploy v3 轻量编排")
	fmt.Println()
	fmt.Println("  deploy init [-f 文件 | --dir 目录] [--stdout] [--overwrite]")
	fmt.Println("  deploy validate [-f 文件...] [--output text|json]")
	fmt.Println("  deploy list [-f 文件...] [--output text|json]")
	fmt.Println("  deploy show <play> [-f 文件...] [目标覆盖]")
	fmt.Println("  deploy plan <play> [-f 文件...] [目标覆盖]")
	fmt.Println("  deploy run <play> [-f 文件...] [目标覆盖] [批量选项] [--check] [--diff] [--yes]")
	fmt.Println()
	fmt.Println("  目标覆盖: --host 主机[,主机] | --tag 标签[,标签] | --all（可配合 --exclude/--exclude-tag 排除）")
	fmt.Println("  批量选项: --serial N --parallel N --fail-fast --max-fail N --max-fail-percent N")
	fmt.Println("  输出选项: --output text|json|ndjson（run 支持 ndjson 事件流）")
	fmt.Println("  变量覆盖: --extra-var key=value")
	fmt.Println("  未使用 -f 时加载 ~/.sshmd/deploy.yaml 与按文件名排序的 ~/.sshmd/deploy.d/*.yaml")
	fmt.Println("  当前目录的 sshmd.deploy.yaml 不会被隐式加载")
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
	extraVars   deploy.Vars
	dir         string
}

func parseDeployCLIOptions(args []string) (deployCLIOptions, error) {
	options := deployCLIOptions{output: "text", extraVars: deploy.Vars{}}
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
		case "--exclude":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.selector.Exclude = append(options.selector.Exclude, item)
			options.hasSelector = true
		case "--exclude-tag":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.selector.ExcludeTags = append(options.selector.ExcludeTags, item)
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
		case "--extra-var", "--var":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			key, val, ok := strings.Cut(item, "=")
			if !ok || strings.TrimSpace(key) == "" {
				return options, fmt.Errorf("%s 需要 key=value 格式", args[i-1])
			}
			options.extraVars[key] = val
		case "--stdout":
			options.stdout = true
		case "--dir":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.dir = item
		case "--overwrite":
			options.overwrite = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return options, fmt.Errorf("未知 deploy 选项: %s", args[i])
			}
			options.positionals = append(options.positionals, args[i])
		}
	}
	if options.output != "text" && options.output != "json" && options.output != "ndjson" {
		return options, fmt.Errorf("--output 必须是 text、json 或 ndjson")
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
		return fmt.Errorf("deploy init 仅支持 -f、--dir、--stdout 和 --overwrite")
	}
	if len(options.files) > 1 {
		return fmt.Errorf("deploy init 只能指定一个 -f")
	}
	if options.dir != "" && (len(options.files) > 0 || options.stdout) {
		return fmt.Errorf("--dir 不能与 -f 或 --stdout 混用")
	}
	if options.dir != "" {
		return app.deployInitDir(options.dir, options.overwrite)
	}
	sample := config.SampleDeployV3
	if options.stdout {
		fmt.Print(sample)
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
	if err := safefile.Write(path, []byte(sample), 0600); err != nil {
		return err
	}
	fmt.Printf("已生成 deploy 配置: %s\n", path)
	fmt.Println()
	fmt.Println("下一步:")
	fileArg := ""
	if len(options.files) == 1 {
		fileArg = " -f " + shellquote.Single(path)
	}
	if !app.hasHostWithAllTags("prod") {
		fmt.Println("  sshmd add web01 root@10.0.0.11 --tags prod")
	}
	fmt.Printf("  sshmd deploy validate%s\n", fileArg)
	fmt.Printf("  sshmd deploy plan update-app%s\n", fileArg)
	return nil
}

// deployInitDir scaffolds a self-contained Deploy demo directory whose
// relative paths (templates/tasks/vars) are consistent with the sample.
func (app *App) deployInitDir(dir string, overwrite bool) error {
	root := config.ExpandPath(dir)
	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(root, "deploy.yaml"), config.DemoDeployV3},
		{filepath.Join(root, "tasks", "prepare.yaml"), config.DemoTasksFile},
		{filepath.Join(root, "vars", "versions.yaml"), config.DemoVarsFile},
		{filepath.Join(root, "README.md"), config.DemoReadme},
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	written := 0
	skipped := 0
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0700); err != nil {
			return err
		}
		if _, err := os.Stat(file.path); err == nil && !overwrite {
			skipped++
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := safefile.Write(file.path, []byte(file.content), 0600); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", file.path, err)
		}
		written++
	}
	fmt.Printf("已生成 Deploy demo 目录: %s（%d 个文件）\n", root, written)
	if skipped > 0 {
		ui.PrintWarn("%d 个文件已存在并跳过；使用 --overwrite 覆盖", skipped)
	}
	fmt.Println()
	fmt.Println("下一步:")
	fileArg := " -f " + shellquote.Single(filepath.Join(root, "deploy.yaml"))
	if !app.hasHostWithAllTags("prod") {
		fmt.Println("  sshmd add web01 root@10.0.0.11 --tags prod")
	}
	fmt.Printf("  sshmd deploy validate%s\n", fileArg)
	fmt.Printf("  sshmd deploy plan update-app%s\n", fileArg)
	fmt.Printf("  sshmd deploy run update-app --check --diff%s --yes\n", fileArg)
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
		options.check || options.diff || len(options.extraVars) > 0
}

package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/deploy"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/safefile"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/ui"
)

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
		return app.cmdDeployShow(args[1:])
	case "plan":
		return app.cmdDeployPlan(args[1:])
	case "run":
		return app.cmdDeployRun(args[1:])
	case "exec":
		return app.cmdDeployExec(args[1:])
	case "copy":
		return app.cmdDeployCopy(args[1:])
	case "help", "-h", "--help":
		app.printDeployHelp()
		return nil
	default:
		return fmt.Errorf("未知 deploy 命令 %q；使用 sshm deploy help 查看帮助", args[0])
	}
}

func (app *App) printDeployHelp() {
	ui.PrintHeader("Deploy 工作流")
	fmt.Println()
	fmt.Println("  deploy init [-f 文件] [--stdout] [--overwrite]")
	fmt.Println("  deploy validate [-f 文件...] [--output text|json]")
	fmt.Println("  deploy list [-f 文件...] [--output text|json]")
	fmt.Println("  deploy show <profile> [-f 文件...] [目标覆盖]")
	fmt.Println("  deploy plan <profile> [-f 文件...] [目标覆盖] [--output text|json]")
	fmt.Println("  deploy run <profile> [-f 文件...] [目标覆盖] [--yes] [--output text|json]")
	fmt.Println("  deploy exec --cmd 命令 [目标] [--yes]")
	fmt.Println("  deploy copy --src 本地 --dest 远端 [目标] [--yes]")
	fmt.Println()
	fmt.Println("  目标: --host 主机[,主机] | --tag 标签[,标签] | --all")
	fmt.Println("  未使用 -f 时依次加载全局 deploy.yaml、deploy.d/*.yaml 和当前目录 sshm.deploy.yaml")
	fmt.Println()
}

type deployCLIOptions struct {
	files       []string
	positionals []string
	output      string
	selector    deploy.TargetSelector
	hasSelector bool
	yes         bool
	mode        string
	parallel    int
	command     string
	src         string
	dest        string
	method      string
	overwrite   bool
	stdout      bool
}

func parseDeployCLIOptions(args []string) (deployCLIOptions, error) {
	options := deployCLIOptions{output: "text", method: "auto"}
	value := func(index *int, flag string) (string, error) {
		if *index+1 >= len(args) {
			return "", fmt.Errorf("%s 缺少值", flag)
		}
		*index = *index + 1
		return args[*index], nil
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
		case "--yes":
			options.yes = true
		case "--mode":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.mode = item
		case "--parallel":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.parallel, err = strconv.Atoi(item)
			if err != nil || options.parallel < 1 {
				return options, fmt.Errorf("--parallel 必须是大于 0 的整数")
			}
		case "--cmd":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.command = item
		case "--src":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.src = item
		case "--dest":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.dest = item
		case "--method":
			item, err := value(&i, args[i])
			if err != nil {
				return options, err
			}
			options.method = item
		case "--overwrite":
			options.overwrite = true
		case "--stdout":
			options.stdout = true
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
	return options, nil
}

func (app *App) cmdDeployInit(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.output != "text" || options.hasSelector || options.yes ||
		options.mode != "" || options.parallel != 0 || options.command != "" || options.src != "" || options.dest != "" ||
		options.method != "auto" {
		return fmt.Errorf("deploy init 仅支持 -f、--stdout 和 --overwrite")
	}
	if len(options.files) > 1 {
		return fmt.Errorf("deploy init 只能指定一个 -f")
	}
	if options.stdout {
		fmt.Print(deploy.Sample)
		return nil
	}
	path := "sshm.deploy.yaml"
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
	return nil
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
	if len(options.positionals) != 0 {
		return fmt.Errorf("用法: sshm deploy validate [-f 文件...]")
	}
	if options.hasSelector || options.yes || options.mode != "" || options.parallel != 0 || options.command != "" ||
		options.src != "" || options.dest != "" || options.method != "auto" || options.overwrite || options.stdout {
		return fmt.Errorf("deploy validate 仅支持 -f 和 --output")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := deploy.ValidateCatalog(catalog, hosts); err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, map[string]any{"ok": true, "sources": catalog.Sources, "profiles": len(catalog.Profiles)})
	}
	fmt.Printf("deploy 配置有效：%d 个文件，%d 个 profile\n", len(catalog.Sources), len(catalog.Profiles))
	return nil
}

func (app *App) cmdDeployList(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if len(options.positionals) != 0 || options.hasSelector || options.yes || options.mode != "" || options.parallel != 0 ||
		options.command != "" || options.src != "" || options.dest != "" || options.method != "auto" || options.overwrite || options.stdout {
		return fmt.Errorf("deploy list 仅支持 -f 和 --output")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := deploy.ValidateCatalog(catalog, hosts); err != nil {
		return err
	}
	if options.output == "json" {
		return deploy.WriteJSON(os.Stdout, catalog.Profiles)
	}
	fmt.Printf("%-28s %-24s %-6s %-8s %-8s %s\n", "NAME", "TARGETS", "STEPS", "MODE", "PARALLEL", "SOURCE")
	for _, profile := range catalog.Profiles {
		fmt.Printf("%-28s %-24s %-6d %-8s %-8d %s\n", profile.Name, profile.Targets.String(), len(profile.Steps),
			profile.Strategy.Mode, profile.Strategy.MaxParallel, profile.Source)
	}
	return nil
}

func (app *App) cmdDeployShow(args []string) error {
	return app.deployPlanCommand(args, true)
}

func (app *App) cmdDeployPlan(args []string) error {
	return app.deployPlanCommand(args, false)
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
	if options.yes || options.command != "" || options.src != "" || options.dest != "" || options.method != "auto" || options.overwrite || options.stdout {
		return fmt.Errorf("deploy plan/show 不支持当前选项")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := validateSelectedDeployProfile(catalog, options.positionals[0], hosts, options); err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, deployOverrides(options))
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
		return fmt.Errorf("用法: sshm deploy run <profile> [-f 文件...] [目标覆盖] [--yes]")
	}
	if options.command != "" || options.src != "" || options.dest != "" || options.method != "auto" || options.overwrite || options.stdout {
		return fmt.Errorf("deploy run 不支持当前选项")
	}
	catalog, hosts, err := app.loadDeploy(options.files)
	if err != nil {
		return err
	}
	if err := validateSelectedDeployProfile(catalog, options.positionals[0], hosts, options); err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(catalog, options.positionals[0], hosts, deployOverrides(options))
	if err != nil {
		return err
	}
	return app.executeDeployPlan(plan, options)
}

func (app *App) cmdDeployExec(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if options.command == "" || !options.hasSelector || len(options.positionals) != 0 {
		return fmt.Errorf("用法: sshm deploy exec --cmd <命令> (--host 主机|--tag 标签|--all) [--yes]")
	}
	if len(options.files) != 0 || options.src != "" || options.dest != "" || options.method != "auto" || options.overwrite || options.stdout {
		return fmt.Errorf("deploy exec 不支持当前选项")
	}
	return app.executeOneShot(options, deploy.Step{Name: "exec", Type: "exec", Command: options.command})
}

func (app *App) cmdDeployCopy(args []string) error {
	options, err := parseDeployCLIOptions(args)
	if err != nil {
		return err
	}
	if options.src == "" || options.dest == "" || !options.hasSelector || len(options.positionals) != 0 {
		return fmt.Errorf("用法: sshm deploy copy --src <本地> --dest <远端> (--host 主机|--tag 标签|--all) [--yes]")
	}
	if len(options.files) != 0 || options.command != "" || options.stdout {
		return fmt.Errorf("deploy copy 不支持当前选项")
	}
	return app.executeOneShot(options, deploy.Step{
		Name: "copy", Type: "copy", Src: options.src, Dest: options.dest,
		Method: options.method, Overwrite: options.overwrite,
	})
}

func (app *App) executeOneShot(options deployCLIOptions, step deploy.Step) error {
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	profile := deploy.Profile{
		Name: "one-shot-" + step.Type, Targets: options.selector, Steps: []deploy.Step{step},
		BaseDir: cwd, Source: "<command>", Vars: map[string]string{},
		Strategy: deploy.Strategy{Mode: "hidden", MaxParallel: 5, ConnectTimeout: deploy.Duration{Duration: 30 * time.Second}, StepTimeout: deploy.Duration{Duration: 15 * time.Minute}},
	}
	if err := deploy.ValidateProfile(profile, hf.Hosts, false); err != nil {
		return err
	}
	catalog := &deploy.Catalog{Sources: []string{"<command>"}, Profiles: []deploy.Profile{profile}, ByName: map[string]deploy.Profile{profile.Name: profile}}
	plan, err := deploy.BuildPlan(catalog, profile.Name, hf.Hosts, deployOverrides(options))
	if err != nil {
		return err
	}
	return app.executeDeployPlan(plan, options)
}

func deployOverrides(options deployCLIOptions) deploy.Overrides {
	overrides := deploy.Overrides{Mode: options.mode, MaxParallel: options.parallel}
	if options.hasSelector {
		overrides.Targets = &options.selector
	}
	return overrides
}

func validateSelectedDeployProfile(catalog *deploy.Catalog, name string, hosts []config.Host, options deployCLIOptions) error {
	profile, ok := catalog.ByName[name]
	if !ok {
		return fmt.Errorf("未找到 deploy profile: %s", name)
	}
	if options.hasSelector {
		profile.Targets = options.selector
	}
	return deploy.ValidateProfile(profile, hosts, false)
}

func writeDeployPlan(plan deploy.Plan, output string) error {
	if output == "json" {
		return deploy.WriteJSON(os.Stdout, plan.JSON())
	}
	deploy.WritePlanText(os.Stdout, plan)
	return nil
}

func (app *App) executeDeployPlan(plan deploy.Plan, options deployCLIOptions) error {
	if options.output == "json" && !options.yes {
		return fmt.Errorf("JSON 模式不会交互确认；执行请显式使用 --yes")
	}
	if !options.yes {
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	executor := app.operationExecutor()
	runner := deploy.Runner{Executor: executor}
	if plan.Strategy.Mode == "visible" {
		runner.Visible = os.Stdout
		if options.output == "json" {
			runner.Visible = os.Stderr
		}
	} else if options.output != "json" {
		runner.Progress = func(done, total int, result deploy.HostResult) {
			fmt.Fprintf(os.Stderr, "\r[%d/%d] 已完成 %-20s", done, total, result.HostAlias)
			if done == total {
				fmt.Fprintln(os.Stderr)
			}
		}
	}
	result := runner.Run(ctx, plan)
	if _, err := deploy.WriteLog(plan, &result); err != nil {
		return fmt.Errorf("写入 deploy 日志失败: %w", err)
	}
	if options.output == "json" {
		if err := deploy.WriteJSON(os.Stdout, result); err != nil {
			return err
		}
	} else {
		deploy.WriteRunText(os.Stdout, result)
	}
	if result.Failed > 0 || result.Cancelled {
		return fmt.Errorf("deploy 执行未完全成功：成功 %d，失败 %d", result.OK, result.Failed)
	}
	return nil
}

package command

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/shellquote"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/sshx"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ui"
)

func (app *App) cmdKeyUse(args []string) error {
	key, hosts, err := app.resolveKeyAndTargets(args)
	if err != nil {
		return err
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, host := range hosts {
		selected[host.ID] = true
	}
	for i := range hf.Hosts {
		if selected[hf.Hosts[i].ID] {
			hf.Hosts[i].Identity = config.ManagedIdentity(key.Name)
			if hf.Hosts[i].Auth == "password" {
				hf.Hosts[i].Auth = "auto"
			}
		}
	}
	if err := app.Store.Save(hf); err != nil {
		return fmt.Errorf("绑定托管密钥失败: %w", err)
	}
	ui.PrintSuccess("已将 %d 台主机绑定到托管密钥 %s", len(hosts), key.Name)
	return nil
}

func (app *App) cmdKeyRemote(args []string, revoke bool) error {
	yes, args := removeFlag(args, "--yes")
	quiet, args := removeFlag(args, "--quiet")
	key, hosts, err := app.resolveKeyAndTargets(args)
	if err != nil {
		return err
	}
	action := "推送"
	subcommand := "push"
	command := installPublicKeyCommand(key.PublicKey)
	if revoke {
		action = "撤销"
		subcommand = "revoke"
		command = revokePublicKeyCommand(key.PublicKey)
	}

	// List affected hosts and ask for confirmation.
	fmt.Println()
	fmt.Printf("即将%s公钥到 %d 台主机:\n", action, len(hosts))
	for _, h := range hosts {
		fmt.Printf("  - %s (%s@%s:%d)\n", h.Alias, h.User, h.Host, h.Port)
	}
	fmt.Println()
	if !yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("远端密钥修改需要确认；非交互环境请使用 --yes")
		}
		if !ui.ReadYesNo(fmt.Sprintf("确认%s? [y/N]: ", action)) {
			ui.PrintWarn("已取消")
			return nil
		}
	}

	return app.runKeyRemoteBatch(action, hosts, command, fmt.Sprintf("sshmd key %s %s", subcommand, key.Name), quiet)
}

func (app *App) cmdKeySetup(args []string) error {
	yes, args := removeFlag(args, "--yes")
	quiet, args := removeFlag(args, "--quiet")
	key, hosts, err := app.resolveKeyAndTargets(args)
	if err != nil {
		return err
	}
	fs, err := app.requireSecretStore()
	if err != nil {
		return err
	}
	if _, err := fs.GetManagedKey(key.Name); err != nil {
		return fmt.Errorf("本地前置检查失败: %w", err)
	}
	if !yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("key setup 需要确认；非交互环境请使用 --yes")
		}
		fmt.Printf("\n即将向 %d 台主机推送、验证并绑定密钥 %s:\n", len(hosts), key.Name)
		for _, host := range hosts {
			fmt.Printf("  - %s (%s@%s:%d)\n", host.Alias, host.User, host.Host, host.Port)
		}
		if !ui.ReadYesNo("确认执行? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}
	if err := app.runKeyRemoteBatch("推送", hosts, installPublicKeyCommand(key.PublicKey), fmt.Sprintf("sshmd key push %s", key.Name), quiet); err != nil {
		return err
	}
	failed := 0
	verifyResults := make([]operation.Result, 0, len(hosts))
	for _, host := range hosts {
		testHost := host
		testHost.Identity = config.ManagedIdentity(key.Name)
		testHost.Auth = "key"
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		output, verifyErr := sshx.CheckPingContext(ctx, testHost, fs)
		cancel()
		result := newOperationResult(host, output, verifyErr, operation.StageExecute,
			fmt.Sprintf("sshmd key setup %s %s --yes", key.Name, host.Alias), time.Since(start))
		verifyResults = append(verifyResults, result)
		if verifyErr != nil {
			failed++
			printOperationFailure(result)
		} else if !quiet {
			ui.PrintSuccess("%s: 托管密钥验证成功", host.Alias)
		}
	}
	if err := writeOperationLog("key-setup-verify", key.Name, verifyResults); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("setup 已推送公钥，但 %d 台主机验证失败，未修改绑定", failed)
	}
	return app.cmdKeyUse(append([]string{key.Name}, hostAliases(hosts)...))
}

func removeFlag(args []string, flag string) (bool, []string) {
	found := false
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == flag {
			found = true
		} else {
			result = append(result, arg)
		}
	}
	return found, result
}

func (app *App) cmdKeyStatus(args []string) error {
	filter := ""
	if len(args) > 0 {
		key, err := app.keyStore().Find(args[0])
		if err != nil {
			return err
		}
		filter = key.Name
	}
	hf, err := app.Store.Load()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("  %-20s %-20s %-10s %s\n", "主机", "托管密钥", "认证", "目标")
	count := 0
	for _, host := range hf.Hosts {
		name, ok := config.ManagedKeyName(host.Identity)
		if !ok || (filter != "" && name != filter) {
			continue
		}
		count++
		fmt.Printf("  %-20s %-20s %-10s %s@%s:%d\n", host.Alias, name, host.Auth, host.User, host.Host, host.Port)
	}
	if count == 0 {
		ui.PrintWarn("没有主机绑定到匹配的托管密钥")
	}
	fmt.Println()
	return nil
}

func (app *App) resolveKeyAndTargets(args []string) (*config.ManagedKey, []config.Host, error) {
	if len(args) < 2 {
		return nil, nil, fmt.Errorf("需要指定密钥和目标；目标支持别名...、--tag 标签、--all（可配合 --exclude/--exclude-tag 排除）")
	}
	key, err := app.keyStore().Find(args[0])
	if err != nil {
		return nil, nil, err
	}
	hosts, err := app.selectHosts(args[1:])
	return key, hosts, err
}

func (app *App) selectHosts(args []string) ([]config.Host, error) {
	hf, err := app.Store.Load()
	if err != nil {
		return nil, err
	}
	selector, err := parseHostSelector(args)
	if err != nil {
		return nil, err
	}
	return selectHostsFrom(hf.Hosts, selector)
}

type hostSelector struct {
	aliases     map[string]bool
	tags        map[string]bool
	all         bool
	exclude     []string
	excludeTags []string
}

func parseHostSelector(args []string) (hostSelector, error) {
	selector := hostSelector{
		aliases: map[string]bool{},
		tags:    map[string]bool{},
	}
	if len(args) == 0 {
		return selector, fmt.Errorf("需要指定目标；目标支持别名...、--tag 标签、--all（可配合 --exclude/--exclude-tag 排除）")
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			selector.all = true
		case "--tag":
			if i+1 >= len(args) {
				return selector, fmt.Errorf("--tag 缺少标签")
			}
			i++
			selector.tags[args[i]] = true
		case "--exclude":
			if i+1 >= len(args) {
				return selector, fmt.Errorf("--exclude 缺少主机")
			}
			i++
			selector.exclude = append(selector.exclude, args[i])
		case "--exclude-tag":
			if i+1 >= len(args) {
				return selector, fmt.Errorf("--exclude-tag 缺少标签")
			}
			i++
			selector.excludeTags = append(selector.excludeTags, args[i])
		default:
			selector.aliases[args[i]] = true
		}
	}
	if selector.all && (len(selector.aliases) > 0 || len(selector.tags) > 0) {
		return selector, fmt.Errorf("--all 不能与具体主机或 --tag 混用")
	}
	return selector, nil
}

func selectHostsFrom(hosts []config.Host, selector hostSelector) ([]config.Host, error) {
	selected, err := selectHostsFromRaw(hosts, selector)
	if err != nil {
		return nil, err
	}
	return deploy.ApplyExcludes(selected, hosts, selector.exclude, selector.excludeTags)
}

func selectHostsFromRaw(hosts []config.Host, selector hostSelector) ([]config.Host, error) {
	aliases := make(map[string]bool, len(selector.aliases))
	for alias := range selector.aliases {
		aliases[alias] = true
	}
	var selected []config.Host
	for index, host := range hosts {
		displayID := strconv.Itoa(index + 1)
		matched := selector.all || aliases[host.Alias] || aliases[host.ID] || aliases[displayID]
		for _, tag := range host.Tags {
			if selector.tags[tag] {
				matched = true
			}
		}
		if matched {
			selected = append(selected, host)
			delete(aliases, host.Alias)
			delete(aliases, host.ID)
			delete(aliases, displayID)
		}
	}
	if len(aliases) > 0 {
		missing := mapKeys(aliases)
		if len(missing) == 1 {
			return nil, missingHostSelectionError(missing[0], hosts)
		}
		return nil, fmt.Errorf("未找到主机: %s；使用 sshmd list 查看全部主机", strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("目标选择结果为空；使用 sshmd list 或 sshmd tag list 查看可用目标")
	}
	return selected, nil
}

type keyRemoteResult struct {
	host     config.Host
	output   string
	err      error
	duration time.Duration
}

func (app *App) runKeyRemoteBatch(action string, hosts []config.Host, command, retryPrefix string, quiet bool) error {
	fs := app.tryGetSecretStore()
	results := make([]keyRemoteResult, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(4, len(hosts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				output, err := sshx.ExecCommandContext(ctx, hosts[i], fs, command)
				cancel()
				results[i] = keyRemoteResult{host: hosts[i], output: output, err: err, duration: time.Since(start)}
			}
		}()
	}
	for i := range hosts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	failed := 0
	logResults := make([]operation.Result, 0, len(results))
	for _, result := range results {
		opResult := newOperationResult(result.host, result.output, result.err, operation.StageExecute,
			fmt.Sprintf("%s %s --yes", retryPrefix, result.host.Alias), result.duration)
		logResults = append(logResults, opResult)
		if result.err != nil {
			failed++
			printOperationFailure(opResult)
		} else if quiet {
			fmt.Printf("%s (%s) ok\n", result.host.Alias, result.host.Host)
		} else {
			ui.PrintSuccess("%s: %s成功", result.host.Alias, action)
		}
	}
	fmt.Printf("%s完成：成功 %d，失败 %d\n", action, len(results)-failed, failed)
	if err := writeOperationLog("key-"+action, action+"远端公钥", logResults); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%s有 %d 台主机失败；请先保存可用密码或配置现有认证", action, failed)
	}
	return nil
}

func installPublicKeyCommand(publicKey string) string {
	key := shellquote.Single(strings.TrimSpace(publicKey))
	return "umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; chmod 700 ~/.ssh; chmod 600 ~/.ssh/authorized_keys; " +
		"grep -qxF -- " + key + " ~/.ssh/authorized_keys || printf '%s\\n' " + key + " >> ~/.ssh/authorized_keys"
}

func revokePublicKeyCommand(publicKey string) string {
	key := shellquote.Single(strings.TrimSpace(publicKey))
	return "if [ -f ~/.ssh/authorized_keys ]; then umask 077; tmp=$(mktemp ~/.ssh/authorized_keys.sshmd.XXXXXX) || exit 1; " +
		"if grep -Fvx -- " + key + " ~/.ssh/authorized_keys > \"$tmp\"; then chmod 600 \"$tmp\" && mv \"$tmp\" ~/.ssh/authorized_keys; " +
		"else rc=$?; if [ \"$rc\" -eq 1 ]; then chmod 600 \"$tmp\" && mv \"$tmp\" ~/.ssh/authorized_keys; else rm -f \"$tmp\"; exit \"$rc\"; fi; fi; fi"
}

func hostAliases(hosts []config.Host) []string {
	result := make([]string, len(hosts))
	for i := range hosts {
		result[i] = hosts[i].Alias
	}
	return result
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

package command

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fanhuadesenlinnn/sshm/internal/config"
	"github.com/fanhuadesenlinnn/sshm/internal/sshx"
	"github.com/fanhuadesenlinnn/sshm/internal/ui"
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
			if hf.Hosts[i].Auth == "password" || hf.Hosts[i].Auth == "system" || hf.Hosts[i].Auth == "ask" {
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
	key, hosts, err := app.resolveKeyAndTargets(args)
	if err != nil {
		return err
	}
	action := "推送"
	command := installPublicKeyCommand(key.PublicKey)
	if revoke {
		action = "撤销"
		command = revokePublicKeyCommand(key.PublicKey)
	}
	return app.runKeyRemoteBatch(action, hosts, command)
}

func (app *App) cmdKeySetup(args []string) error {
	key, hosts, err := app.resolveKeyAndTargets(args)
	if err != nil {
		return err
	}
	if err := app.runKeyRemoteBatch("推送", hosts, installPublicKeyCommand(key.PublicKey)); err != nil {
		return err
	}
	fs, err := app.requireSecretStore()
	if err != nil {
		return err
	}
	failed := 0
	for _, host := range hosts {
		testHost := host
		testHost.Identity = config.ManagedIdentity(key.Name)
		testHost.Auth = "key"
		ok, msg := sshx.CheckPing(testHost, fs)
		if !ok {
			failed++
			ui.PrintError("%s: 托管密钥验证失败: %s", host.Alias, strings.TrimSpace(msg))
		}
	}
	if failed > 0 {
		return fmt.Errorf("setup 已推送公钥，但 %d 台主机验证失败，未修改绑定", failed)
	}
	return app.cmdKeyUse(append([]string{key.Name}, hostAliases(hosts)...))
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
		return nil, nil, fmt.Errorf("需要指定密钥和目标；目标支持别名...、--group 分组、--tag 标签、--all")
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
	aliases := map[string]bool{}
	groups := map[string]bool{}
	tags := map[string]bool{}
	all := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			all = true
		case "--group":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--group 缺少分组名")
			}
			i++
			groups[args[i]] = true
		case "--tag":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--tag 缺少标签")
			}
			i++
			tags[args[i]] = true
		default:
			aliases[args[i]] = true
		}
	}
	var selected []config.Host
	for index, host := range hf.Hosts {
		displayID := strconv.Itoa(index + 1)
		matched := all || aliases[host.Alias] || aliases[host.ID] || aliases[displayID]
		if groups[host.Group] {
			matched = true
		}
		for _, tag := range host.Tags {
			if tags[tag] {
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
		return nil, fmt.Errorf("未找到主机: %s", strings.Join(mapKeys(aliases), ", "))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("目标选择结果为空")
	}
	return selected, nil
}

type keyRemoteResult struct {
	host config.Host
	err  error
}

func (app *App) runKeyRemoteBatch(action string, hosts []config.Host, command string) error {
	fs := app.tryGetSecretStore()
	results := make([]keyRemoteResult, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(4, len(hosts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, err := sshx.ExecCommandContext(ctx, hosts[i], fs, command)
				cancel()
				results[i] = keyRemoteResult{host: hosts[i], err: err}
			}
		}()
	}
	for i := range hosts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	failed := 0
	for _, result := range results {
		if result.err != nil {
			failed++
			ui.PrintError("%s: %s失败: %v", result.host.Alias, action, result.err)
		} else {
			ui.PrintSuccess("%s: %s成功", result.host.Alias, action)
		}
	}
	fmt.Printf("%s完成：成功 %d，失败 %d\n", action, len(results)-failed, failed)
	if failed > 0 {
		return fmt.Errorf("%s有 %d 台主机失败；请先保存可用密码或配置现有认证", action, failed)
	}
	return nil
}

func installPublicKeyCommand(publicKey string) string {
	key := shellSingleQuote(strings.TrimSpace(publicKey))
	return "umask 077; mkdir -p ~/.ssh; touch ~/.ssh/authorized_keys; chmod 700 ~/.ssh; chmod 600 ~/.ssh/authorized_keys; " +
		"grep -qxF -- " + key + " ~/.ssh/authorized_keys || printf '%s\\n' " + key + " >> ~/.ssh/authorized_keys"
}

func revokePublicKeyCommand(publicKey string) string {
	key := shellSingleQuote(strings.TrimSpace(publicKey))
	return "if [ -f ~/.ssh/authorized_keys ]; then umask 077; tmp=$(mktemp ~/.ssh/authorized_keys.sshm.XXXXXX); " +
		"grep -Fvx -- " + key + " ~/.ssh/authorized_keys > \"$tmp\" || true; chmod 600 \"$tmp\"; mv \"$tmp\" ~/.ssh/authorized_keys; fi"
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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

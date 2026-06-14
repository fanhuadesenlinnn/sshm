package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/ui"
)

func (app *App) cmdTag(args []string) error {
	if len(args) == 0 {
		if ui.IsTerminal() {
			return app.tagCenter()
		}
		app.printTagHelp()
		return nil
	}
	switch args[0] {
	case "list", "ls", "l":
		return app.cmdTagList(args[1:])
	case "show", "s":
		return app.cmdTagShow(args[1:])
	case "create", "new", "c":
		return app.cmdTagCreate(args[1:])
	case "edit", "e":
		return app.cmdTagEdit(args[1:])
	case "rename":
		return app.cmdTagRename(args[1:])
	case "delete", "del", "rm", "d":
		return app.cmdTagDelete(args[1:])
	case "add", "a":
		return app.cmdTagAdd(args[1:])
	case "remove", "r":
		return app.cmdTagRemove(args[1:])
	case "set":
		return app.cmdTagSet(args[1:])
	case "clear":
		return app.cmdTagClear(args[1:])
	case "help", "h":
		app.printTagHelp()
		return nil
	default:
		return fmt.Errorf("未知 tag 命令 %q；使用 sshm tag help 查看帮助", args[0])
	}
}

func (app *App) tagCenter() error {
	app.printTagHelp()
	for {
		parts := parseArgs(ui.ReadLine(ui.CyanText("tag> ")))
		if len(parts) == 0 {
			continue
		}
		if parts[0] == "q" || parts[0] == "quit" || parts[0] == "back" {
			return nil
		}
		if err := app.cmdTag(parts); err != nil {
			fmt.Fprintln(os.Stderr, ui.ErrorMsg("%v", err))
		}
	}
}

func (app *App) printTagHelp() {
	ui.PrintHeader("标签管理中心")
	fmt.Println()
	fmt.Println("  l/list                         列出标签与使用统计")
	fmt.Println("  s/show <标签|--untagged>       查看标签关联主机")
	fmt.Println("  c/create <标签> [--note 备注]  创建标签")
	fmt.Println("  e/edit <标签> [--note 备注]    修改标签备注")
	fmt.Println("  rename <旧标签> <新标签>       重命名标签及所有引用")
	fmt.Println("  d/delete <标签...> [--yes]     删除标签及所有引用")
	fmt.Println("  a/add <标签> <目标...>         为主机添加标签")
	fmt.Println("  r/remove <标签> <目标...>      从主机移除标签")
	fmt.Println("  set <目标...> --tags <标签>    替换主机的全部标签")
	fmt.Println("  clear <目标...>                清空主机标签")
	fmt.Println()
	fmt.Println("  目标支持：别名...、--tag 标签、--all")
	fmt.Println("  输入 back/q 返回主命令页")
	fmt.Println()
}

func (app *App) cmdTagList(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("用法: sshm tag list")
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return err
	}
	categorized := 0
	for _, host := range doc.Hosts {
		if len(host.Tags) > 0 {
			categorized++
		}
	}
	ui.PrintHeader(fmt.Sprintf("标签 · %d 个，已分类 %d 台，未分类 %d 台", len(doc.Tags.Items), categorized, len(doc.Hosts)-categorized))
	ui.RenderTagsTable(doc.Tags.Items, doc.Hosts)
	return nil
}

func (app *App) cmdTagShow(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法: sshm tag show <标签|--untagged>")
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return err
	}
	name := args[0]
	note := ""
	if name != "--untagged" {
		tag, ok := doc.Tags.Find(name)
		if !ok {
			return fmt.Errorf("未找到标签: %s", name)
		}
		note = tag.Note
	}
	var hosts []config.Host
	var indices []int
	for i, host := range doc.Hosts {
		matched := name == "--untagged" && len(host.Tags) == 0
		if name != "--untagged" {
			matched = host.HasTag(name)
		}
		if matched {
			hosts = append(hosts, host)
			indices = append(indices, i)
		}
	}
	title := fmt.Sprintf("标签 %s · %d 台", name, len(hosts))
	if name == "--untagged" {
		title = fmt.Sprintf("未分类主机 · %d 台", len(hosts))
	}
	ui.PrintHeader(title)
	if note != "" {
		fmt.Printf("  备注: %s\n", note)
	}
	ui.RenderHostsTableWithOptions(hosts, ui.HostTableOptions{Indices: indices})
	return nil
}

func (app *App) cmdTagCreate(args []string) error {
	name, note, err := parseTagNameAndNote(args)
	if err != nil {
		return err
	}
	if err := app.Store.Repository().Update(func(doc *config.Document) error {
		if _, ok := doc.Tags.Find(name); ok {
			return fmt.Errorf("标签 %q 已存在", name)
		}
		doc.Tags.Items = append(doc.Tags.Items, config.Tag{Name: name, Note: note})
		return nil
	}); err != nil {
		return err
	}
	ui.PrintSuccess("已创建标签：%s", name)
	return nil
}

func (app *App) cmdTagEdit(args []string) error {
	name, note, hasNote, err := parseTagEditArgs(args)
	if err != nil {
		return err
	}
	if !hasNote {
		doc, loadErr := app.Store.Repository().Load()
		if loadErr != nil {
			return loadErr
		}
		tag, ok := doc.Tags.Find(name)
		if !ok {
			return fmt.Errorf("未找到标签: %s", name)
		}
		if !ui.IsTerminal() {
			return fmt.Errorf("非交互环境请使用 --note 指定备注")
		}
		note = readEditableValue("标签备注", tag.Note)
	}
	if err := app.Store.Repository().Update(func(doc *config.Document) error {
		tag, ok := doc.Tags.Find(name)
		if !ok {
			return fmt.Errorf("未找到标签: %s", name)
		}
		tag.Note = note
		return nil
	}); err != nil {
		return err
	}
	ui.PrintSuccess("已更新标签：%s", name)
	return nil
}

func (app *App) cmdTagRename(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("用法: sshm tag rename <旧标签> <新标签>")
	}
	oldName, newName := args[0], args[1]
	if err := config.ValidateTagName(newName); err != nil {
		return err
	}
	updatedHosts := 0
	if err := app.Store.Repository().Update(func(doc *config.Document) error {
		tag, ok := doc.Tags.Find(oldName)
		if !ok {
			return fmt.Errorf("未找到标签: %s", oldName)
		}
		if _, exists := doc.Tags.Find(newName); exists {
			return fmt.Errorf("标签 %q 已存在", newName)
		}
		tag.Name = newName
		for i := range doc.Hosts {
			if replaceHostTag(&doc.Hosts[i], oldName, newName) {
				updatedHosts++
			}
		}
		return nil
	}); err != nil {
		return err
	}
	ui.PrintSuccess("已将标签 %s 重命名为 %s，并更新 %d 台主机", oldName, newName, updatedHosts)
	return nil
}

func (app *App) cmdTagDelete(args []string) error {
	yes, names := removeFlag(args, "--yes")
	if len(names) == 0 {
		return fmt.Errorf("用法: sshm tag delete <标签...> [--yes]")
	}
	remove := map[string]bool{}
	for _, name := range names {
		if err := config.ValidateTagName(name); err != nil {
			return err
		}
		remove[name] = true
	}
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return err
	}
	for name := range remove {
		if _, ok := doc.Tags.Find(name); !ok {
			return fmt.Errorf("未找到标签: %s", name)
		}
	}
	var affected []string
	for _, host := range doc.Hosts {
		for _, tag := range host.Tags {
			if remove[tag] {
				affected = append(affected, host.Alias)
				break
			}
		}
	}
	if len(affected) > 0 && !yes {
		if !ui.IsTerminal() {
			return fmt.Errorf("删除会清除 %d 台主机的标签引用；非交互环境请使用 --yes", len(affected))
		}
		fmt.Printf("删除会清除 %d 台主机的标签引用: %s\n", len(affected), strings.Join(affected, ", "))
		if !ui.ReadYesNo("确认删除? [y/N]: ") {
			ui.PrintWarn("已取消")
			return nil
		}
	}
	if err := app.Store.Repository().Update(func(doc *config.Document) error {
		found := map[string]bool{}
		filtered := doc.Tags.Items[:0]
		for _, tag := range doc.Tags.Items {
			if remove[tag.Name] {
				found[tag.Name] = true
			} else {
				filtered = append(filtered, tag)
			}
		}
		for name := range remove {
			if !found[name] {
				return fmt.Errorf("未找到标签: %s", name)
			}
		}
		doc.Tags.Items = filtered
		for i := range doc.Hosts {
			doc.Hosts[i].Tags = removeHostTags(doc.Hosts[i].Tags, remove)
		}
		return nil
	}); err != nil {
		return err
	}
	ui.PrintSuccess("已删除标签：%s", strings.Join(names, ", "))
	return nil
}

func (app *App) cmdTagAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm tag add <标签> <目标...>")
	}
	name := args[0]
	if err := config.ValidateTagName(name); err != nil {
		return err
	}
	selected, changed, err := app.updateSelectedHosts(args[1:], func(doc *config.Document) error {
		doc.Tags.Ensure(name)
		return nil
	}, func(host *config.Host) bool {
		if host.HasTag(name) {
			return false
		}
		host.Tags = append(host.Tags, name)
		return true
	})
	if err != nil {
		return err
	}
	ui.PrintSuccess("已为 %d/%d 台主机添加标签 %s", changed, selected, name)
	return nil
}

func (app *App) cmdTagRemove(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("用法: sshm tag remove <标签> <目标...>")
	}
	name := args[0]
	if err := config.ValidateTagName(name); err != nil {
		return err
	}
	selected, changed, err := app.updateSelectedHosts(args[1:], func(doc *config.Document) error {
		if _, ok := doc.Tags.Find(name); !ok {
			return fmt.Errorf("未找到标签: %s", name)
		}
		return nil
	}, func(host *config.Host) bool {
		if !host.HasTag(name) {
			return false
		}
		host.Tags = removeHostTags(host.Tags, map[string]bool{name: true})
		return true
	})
	if err != nil {
		return err
	}
	ui.PrintSuccess("已从 %d/%d 台主机移除标签 %s", changed, selected, name)
	return nil
}

func (app *App) cmdTagSet(args []string) error {
	targets, tags, err := parseTagSetArgs(args)
	if err != nil {
		return err
	}
	selected, changed, err := app.updateSelectedHosts(targets, func(doc *config.Document) error {
		doc.Tags.Ensure(tags...)
		return nil
	}, func(host *config.Host) bool {
		if equalStrings(host.Tags, tags) {
			return false
		}
		host.Tags = append([]string(nil), tags...)
		return true
	})
	if err != nil {
		return err
	}
	ui.PrintSuccess("已替换 %d/%d 台主机的标签", changed, selected)
	return nil
}

func (app *App) cmdTagClear(args []string) error {
	selected, changed, err := app.updateSelectedHosts(args, nil, func(host *config.Host) bool {
		if len(host.Tags) == 0 {
			return false
		}
		host.Tags = []string{}
		return true
	})
	if err != nil {
		return err
	}
	ui.PrintSuccess("已清空 %d/%d 台主机的标签", changed, selected)
	return nil
}

func (app *App) updateSelectedHosts(
	selectorArgs []string,
	prepare func(*config.Document) error,
	mutate func(*config.Host) bool,
) (selectedCount, changedCount int, err error) {
	selector, err := parseHostSelector(selectorArgs)
	if err != nil {
		return 0, 0, err
	}
	err = app.Store.Repository().Update(func(doc *config.Document) error {
		selected, selectErr := selectHostsFrom(doc.Hosts, selector)
		if selectErr != nil {
			return selectErr
		}
		if prepare != nil {
			if err := prepare(doc); err != nil {
				return err
			}
		}
		selectedIDs := make(map[string]bool, len(selected))
		for _, host := range selected {
			selectedIDs[host.ID] = true
		}
		selectedCount = len(selected)
		for i := range doc.Hosts {
			if selectedIDs[doc.Hosts[i].ID] && mutate(&doc.Hosts[i]) {
				changedCount++
			}
		}
		return nil
	})
	return selectedCount, changedCount, err
}

func parseTagNameAndNote(args []string) (name, note string, err error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("用法: sshm tag create <标签> [--note 备注]")
	}
	name = args[0]
	if err := config.ValidateTagName(name); err != nil {
		return "", "", err
	}
	for i := 1; i < len(args); i++ {
		if args[i] != "--note" || i+1 >= len(args) {
			return "", "", fmt.Errorf("用法: sshm tag create <标签> [--note 备注]")
		}
		note = args[i+1]
		i++
	}
	return name, note, nil
}

func parseTagEditArgs(args []string) (name, note string, hasNote bool, err error) {
	if len(args) == 0 {
		return "", "", false, fmt.Errorf("用法: sshm tag edit <标签> [--note 备注]")
	}
	name = args[0]
	if err := config.ValidateTagName(name); err != nil {
		return "", "", false, err
	}
	if len(args) == 1 {
		return name, "", false, nil
	}
	if len(args) != 3 || args[1] != "--note" {
		return "", "", false, fmt.Errorf("用法: sshm tag edit <标签> [--note 备注]")
	}
	return name, args[2], true, nil
}

func parseTagSetArgs(args []string) ([]string, []string, error) {
	var targets []string
	var tags []string
	found := false
	for i := 0; i < len(args); i++ {
		if args[i] != "--tags" {
			targets = append(targets, args[i])
			continue
		}
		if found || i+1 >= len(args) {
			return nil, nil, fmt.Errorf("用法: sshm tag set <目标...> --tags <标签>")
		}
		found = true
		i++
		tags = config.ParseTags(args[i])
	}
	if len(targets) == 0 || !found || len(tags) == 0 {
		return nil, nil, fmt.Errorf("用法: sshm tag set <目标...> --tags <标签>")
	}
	for _, tag := range tags {
		if err := config.ValidateTagName(tag); err != nil {
			return nil, nil, err
		}
	}
	return targets, tags, nil
}

func replaceHostTag(host *config.Host, oldName, newName string) bool {
	changed := false
	for i := range host.Tags {
		if host.Tags[i] == oldName {
			host.Tags[i] = newName
			changed = true
		}
	}
	host.Tags = config.NormalizeTags(host.Tags)
	return changed
}

func removeHostTags(tags []string, remove map[string]bool) []string {
	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !remove[tag] {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

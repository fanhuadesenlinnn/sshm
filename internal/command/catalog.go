package command

type commandGroupMeta struct {
	ID    string
	Title string
}

const (
	commandGroupDaily   = "daily"
	commandGroupHost    = "host"
	commandGroupOps     = "ops"
	commandGroupSecrets = "secrets"
	commandGroupConfig  = "config"
)

var commandGroups = []commandGroupMeta{
	{ID: commandGroupDaily, Title: "常用入口"},
	{ID: commandGroupHost, Title: "主机与标签"},
	{ID: commandGroupOps, Title: "连接、执行与传输"},
	{ID: commandGroupSecrets, Title: "凭据与密钥"},
	{ID: commandGroupConfig, Title: "配置、日志与工具"},
}

type commandCatalogEntry struct {
	Name          string
	Aliases       []string
	Group         string
	Short         string
	HelpUsage     string
	HelpSummary   string
	LegacyOptions []string
}

var commandCatalog = []commandCatalogEntry{
	{Name: "list", Aliases: []string{"ls"}, Group: commandGroupDaily, Short: "列出主机", HelpUsage: "list [--compact|--wide]", HelpSummary: "列出所有主机", LegacyOptions: []string{"--list", "-l"}},
	{Name: "find-con", Aliases: []string{"f", "pick"}, Group: commandGroupDaily, Short: "查找并连接主机", HelpUsage: "find-con", HelpSummary: "打开可搜索主机选择器"},
	{Name: "recent", Group: commandGroupDaily, Short: "显示收藏和最近连接", HelpUsage: "recent [数量]", HelpSummary: "显示收藏和最近连接"},
	{Name: "connect", Aliases: []string{"conn"}, Group: commandGroupDaily, Short: "连接主机", HelpUsage: "connect <别名|ID>", HelpSummary: "显式连接主机"},
	{Name: "host", Group: commandGroupHost, Short: "管理主机", HelpUsage: "host", HelpSummary: "进入主机管理中心"},
	{Name: "add", Group: commandGroupHost, Short: "添加主机", HelpUsage: "add [别名 user@主机[:端口]]", HelpSummary: "添加主机", LegacyOptions: []string{"--add", "-a"}},
	{Name: "add-batch", Group: commandGroupHost, Short: "批量添加主机", HelpUsage: "add-batch [别名=目标...]", HelpSummary: "批量添加主机"},
	{Name: "edit", Group: commandGroupHost, Short: "编辑主机", HelpUsage: "edit <别名|ID>", HelpSummary: "编辑主机", LegacyOptions: []string{"--edit", "-e"}},
	{Name: "delete", Aliases: []string{"del", "rm"}, Group: commandGroupHost, Short: "删除主机", HelpUsage: "delete <别名|ID> [--yes]", HelpSummary: "删除主机", LegacyOptions: []string{"--delete", "--del", "-d"}},
	{Name: "show", Aliases: []string{"info"}, Group: commandGroupHost, Short: "显示主机详情", HelpUsage: "show <别名|ID>", HelpSummary: "显示主机详情", LegacyOptions: []string{"--show"}},
	{Name: "search", Aliases: []string{"find"}, Group: commandGroupHost, Short: "搜索主机", HelpUsage: "search <关键词...>", HelpSummary: "搜索主机", LegacyOptions: []string{"--search", "--find", "-s"}},
	{Name: "tag", Aliases: []string{"tags"}, Group: commandGroupHost, Short: "管理标签", HelpUsage: "tag", HelpSummary: "进入标签管理中心", LegacyOptions: []string{"--tag"}},
	{Name: "pin", Group: commandGroupHost, Short: "收藏主机", HelpUsage: "pin <别名|ID>", HelpSummary: "收藏主机"},
	{Name: "unpin", Group: commandGroupHost, Short: "取消收藏主机", HelpUsage: "unpin <别名|ID>", HelpSummary: "取消收藏主机"},
	{Name: "ping", Aliases: []string{"p"}, Group: commandGroupOps, Short: "测试 SSH 连接", HelpUsage: "ping [--yes] [--quiet] [目标]", HelpSummary: "测试连接", LegacyOptions: []string{"--ping", "-p"}},
	{Name: "exec", Group: commandGroupOps, Short: "在单台主机执行命令", HelpUsage: "exec [--yes] [--quiet] [--no-log] ...", HelpSummary: "执行命令", LegacyOptions: []string{"--exec", "-x"}},
	{Name: "exec-tag", Group: commandGroupOps, Short: "按标签批量执行命令", HelpUsage: "exec-tag [批量选项] <标签|all> [--] <命令>", HelpSummary: "按标签执行命令", LegacyOptions: []string{"--exec-tag", "--xt"}},
	{Name: "push", Group: commandGroupOps, Short: "向单台主机推送文件或目录", HelpUsage: "push <主机> <本地> <远程> [选项]", HelpSummary: "推送文件或目录"},
	{Name: "pull", Group: commandGroupOps, Short: "从单台主机拉取文件或目录", HelpUsage: "pull <主机> <远程> <本地> [选项]", HelpSummary: "拉取文件或目录"},
	{Name: "push-tag", Group: commandGroupOps, Short: "按标签批量推送文件或目录", HelpUsage: "push-tag <标签|all> <本地> <远程> [选项]", HelpSummary: "按标签推送文件或目录"},
	{Name: "pull-tag", Group: commandGroupOps, Short: "按标签批量拉取文件或目录", HelpUsage: "pull-tag <标签|all> <远程> <本地> [选项]", HelpSummary: "按标签拉取文件或目录"},
	{Name: "forward", Group: commandGroupOps, Short: "建立本地端口转发", HelpUsage: "forward <主机> <本地> <远程>", HelpSummary: "建立本地端口转发"},
	{Name: "deploy", Group: commandGroupOps, Short: "运行 Deploy v2 轻量编排", HelpUsage: "deploy", HelpSummary: "批量部署工作流"},
	{Name: "key", Aliases: []string{"k"}, Group: commandGroupSecrets, Short: "管理托管密钥", HelpUsage: "key", HelpSummary: "进入托管密钥中心"},
	{Name: "passwd", Group: commandGroupSecrets, Short: "设置 SSH 密码", HelpUsage: "passwd <目标...|--tag 标签|--all> [--yes]", HelpSummary: "设置 SSH 密码", LegacyOptions: []string{"--passwd"}},
	{Name: "forget-pass", Group: commandGroupSecrets, Short: "删除 SSH 密码", HelpUsage: "forget-pass <目标...|--tag 标签|--all> [--yes]", HelpSummary: "删除 SSH 密码", LegacyOptions: []string{"--forget-pass"}},
	{Name: "show-pubkey", Group: commandGroupSecrets, Short: "显示托管公钥", HelpUsage: "show-pubkey <别名|ID>", HelpSummary: "显示公钥", LegacyOptions: []string{"--show-pubkey"}},
	{Name: "auth", Group: commandGroupSecrets, Short: "修改认证策略", HelpUsage: "auth <别名|ID>", HelpSummary: "修改认证策略", LegacyOptions: []string{"--auth"}},
	{Name: "lock", Group: commandGroupSecrets, Short: "锁定当前会话密码库", HelpUsage: "lock", HelpSummary: "锁定当前会话密码库", LegacyOptions: []string{"--lock"}},
	{Name: "init", Group: commandGroupConfig, Short: "初始化 ~/.sshm 工作目录和 v2 配置文件", HelpUsage: "init", HelpSummary: "初始化 ~/.sshm 工作目录"},
	{Name: "config", Group: commandGroupConfig, Short: "查看和编辑主配置", HelpUsage: "config path/edit", HelpSummary: "查看路径或校验后编辑 sshm.yaml"},
	{Name: "doctor", Group: commandGroupConfig, Short: "检查配置与凭据环境", HelpUsage: "doctor", HelpSummary: "检查配置与凭据环境"},
	{Name: "logs", Group: commandGroupConfig, Short: "查看或清理操作日志", HelpUsage: "logs [clean --yes]", HelpSummary: "查看或清理操作日志"},
	{Name: "export-ssh-config", Group: commandGroupConfig, Short: "导出 OpenSSH 配置", HelpUsage: "export-ssh-config <文件>", HelpSummary: "导出 SSH 配置", LegacyOptions: []string{"--export-ssh-config"}},
	{Name: "import-ssh-config", Group: commandGroupConfig, Short: "导入 OpenSSH 配置", HelpUsage: "import-ssh-config [文件]", HelpSummary: "导入 SSH 配置", LegacyOptions: []string{"--import-ssh-config"}},
	{Name: "completion", Group: commandGroupConfig, Short: "生成 Shell 自动补全脚本", HelpUsage: "completion <bash|zsh|fish>", HelpSummary: "生成 Shell 自动补全脚本"},
}

func commandEntry(name string) (commandCatalogEntry, bool) {
	for _, entry := range commandCatalog {
		if entry.Name == name {
			return entry, true
		}
		for _, alias := range entry.Aliases {
			if alias == name {
				return entry, true
			}
		}
	}
	return commandCatalogEntry{}, false
}

func commandShort(name, fallback string) string {
	if entry, ok := commandEntry(name); ok && entry.Short != "" {
		return entry.Short
	}
	return fallback
}

func commandAliases(name string) []string {
	if entry, ok := commandEntry(name); ok {
		return append([]string(nil), entry.Aliases...)
	}
	return nil
}

func commandGroupID(name string) string {
	if entry, ok := commandEntry(name); ok {
		return entry.Group
	}
	return ""
}

func commandNamesForCompletion() []string {
	names := []string{"help"}
	for _, entry := range commandCatalog {
		names = append(names, entry.Name)
		names = append(names, entry.Aliases...)
	}
	return names
}

func legacyOptionCandidates() []string {
	var options []string
	for _, entry := range commandCatalog {
		options = append(options, entry.LegacyOptions...)
	}
	options = append(options, "--help", "--version")
	return options
}

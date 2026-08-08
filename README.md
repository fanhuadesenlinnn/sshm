# sshmd v6.2.2

sshmd 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。它使用 Go 原生 SSH 能力管理主机、标签、凭据、批量命令、安全文件传输和 Deploy v3 编排。

> 版本说明：产品发布版本是 `v6.2.2`，Go module 是 `github.com/fanhuadesenlinnn/sshmd/v6`，主配置 schema 为 `version: 2`，Deploy 配置 schema 为 `version: 3`。

## 安装

### 一键安装

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/fanhuadesenlinnn/sshmd/main/scripts/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/fanhuadesenlinnn/sshmd/main/scripts/install.ps1 | iex
```

安装器会自动识别操作系统和 AMD64/ARM64 架构，从最新 GitHub Release 下载对应制品，使用 `checksums.txt` 验证 SHA-256，然后安装并执行 `sshmd --version`。安装脚本只保存在代码仓库中，不会作为 Release 附件发布。

macOS/Linux 默认安装到 `/usr/local/bin`，权限不足时会请求 `sudo`；也可以指定版本或安装目录：

```bash
curl -fsSL https://raw.githubusercontent.com/fanhuadesenlinnn/sshmd/main/scripts/install.sh | \
  sh -s -- --version v6.2.2 --install-dir "$HOME/.local/bin"
```

Windows 默认安装到 `%LOCALAPPDATA%\Programs\sshmd` 并加入用户 PATH。

### 使用 Go 安装

已经安装 Go 1.25 或更高版本时，也可以使用：

```bash
go install github.com/fanhuadesenlinnn/sshmd/v6@latest
```

如果 `proxy.golang.org` 访问较慢，可临时指定国内代理：

```bash
GOPROXY=https://goproxy.cn,direct go install github.com/fanhuadesenlinnn/sshmd/v6@latest
```

PowerShell：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"; go install github.com/fanhuadesenlinnn/sshmd/v6@latest
```

也可以前往 [GitHub Releases](https://github.com/fanhuadesenlinnn/sshmd/releases/latest) 手工下载对应平台的压缩包和校验文件。

## 配置约定

- 默认数据目录只使用 `~/.sshmd`，唯一可用的路径覆盖变量是 `SSHMD_HOME`。
- 不支持 `SSHMD_CONFIG_FILE`；主配置固定为 `~/.sshmd/sshmd.yaml`。
- 主配置使用 `version: 2` schema。
- Deploy 仅支持 schema v3 playbook；`version: 2` 文件不会被加载。
- `all` 是虚拟标签，`--all` 不能与具体主机或标签选择器混用。
- 当前目录的 `sshmd.deploy.yaml` 不会被隐式加载，项目文件必须通过 `--file` 指定。

## 首次使用

```bash
sshmd
sshmd init
sshmd config path
sshmd doctor
```

未初始化时直接运行 `sshmd` 会显示首次使用引导，不会直接进入空工作台。

`sshmd init` 创建：

```text
~/.sshmd/
├── sshmd.yaml
├── deploy.yaml
├── deploy.d/
├── templates/
├── README.md
├── logs/
├── backups/
└── tmp/
```

`sshmd.yaml` 会包含快速开始、字段用途、主机示例和安全边界说明。`deploy.yaml` 默认不启用任何工作流，提供一份完全注释掉的 Deploy v3 示例（快速开始、全部模块、register/when、block/rescue、include、sleep/confirm）；可以安全地先运行 `sshmd deploy validate`，再按注释创建 play。`templates/` 含一个可运行的模板示例，`README.md` 是一页速查。已有 `deploy.yaml` 不会被 `sshmd init --force` 覆盖。

主配置、日志、deploy 文件和备份都以同一个 `SSHMD_HOME` 为根目录。发现旧配置时，`init` 与 `doctor` 只输出警告。

## 常用命令

```bash
sshmd                         # 打开轻量工作台
sshmd web01                   # 按别名或 ID 直连
sshmd list
sshmd add web01 root@10.0.0.11
sshmd edit web01
sshmd tag
sshmd ping web01
sshmd passwd web01 web02
sshmd passwd --tag prod
sshmd exec web01 "uptime"
sshmd exec-tag prod "uptime"
sshmd exec-tag all "uptime"
```

Cobra 提供根命令和 Deploy 子命令帮助：

```bash
sshmd --help
sshmd host --help
sshmd key --help
sshmd tag --help
sshmd deploy --help
sshmd deploy run --help
```

在交互工作台中，复杂远程命令建议用 `--` 明确标记起点；`--` 后的文本不再由 sshmd 拆分或重组：

```text
sshmd> x web01 --quiet -- pwd
sshmd> x web01 -- awk '{print $1}' /tmp/data
sshmd> xt prod --parallel 4 --yes -- systemctl restart app
```

不写 `--` 时仍兼容 `x web01 pwd` 和 `x web01 --quiet pwd`。只有远程命令开始前的已知 sshmd 选项会被解析；一旦识别到命令起点，后续引号、变量、反斜杠和命令参数都原样传给远程 Shell。

## 批量执行

`exec-tag`、`push-tag`、`pull-tag` 和 `deploy run` 复用同一个 BatchRunner。

```bash
sshmd exec-tag prod "systemctl status app" \
  --serial 2 \
  --parallel 2 \
  --timeout 30s \
  --connect-timeout 10s \
  --max-fail-percent 25 \
  --yes
```

通用参数：

```text
--parallel N
--serial N
--timeout 30s|5m|1h|7d
--connect-timeout 10s
--fail-fast
--max-fail N
--max-fail-percent N
--exclude 主机[,主机]
--exclude-tag 标签[,标签]
--yes
--no-log
--quiet
```

`--exclude`/`--exclude-tag` 在目标解析后应用，可用于 `exec-tag`、`push-tag`、`pull-tag` 与 `deploy run`/`plan`（如 `exec-tag all --exclude-tag legacy`）。被排除的主机或标签必须存在于主机清单，拼写错误会直接报错而不是静默漏排除。

结果状态为 `ok`、`changed`、`would-change`、`failed`、`unreachable`、`skipped`。`Ctrl+C` 会取消运行中的 context，未开始目标标记为 `skipped`。

退出码：

```text
0    全部成功，或仅有预期状态
1    failed，或失败策略产生 skipped
2    无 failed，但存在 unreachable
3    参数或配置错误
4    执行开始前被 vault/auth 阻断
130  用户中断
```

## 安全文件传输

单主机：

```bash
sshmd push web01 ./dist/app.tar.gz /opt/app/app.tar.gz
sshmd pull web01 /etc/nginx/nginx.conf ./nginx.web01.conf
```

按标签：

```bash
sshmd push-tag prod ./dist/app.tar.gz /opt/app/app.tar.gz --backup --yes
sshmd pull-tag prod /etc/nginx ./backup --yes
sshmd pull-tag all /etc/hosts ./backup --flat --yes
```

传输语义：

- 文件默认使用 SHA-256；目录按逐文件 manifest 比较并聚合状态。
- 默认拒绝覆盖不同内容；`--overwrite` 与 `--backup` 互斥。
- push/pull 先写临时目标，校验后再 rename，不直接写最终文件。
- 远端路径必须是明确路径，拒绝 `~`、根路径、空路径和上级目录组件。
- 拒绝符号链接、设备文件、socket、FIFO 等特殊文件。
- 多主机 pull 默认保存为 `local/host_alias/remote-path`。
- `--flat` 会在执行前检查本次操作内部的目标冲突。
- `auto` 可使用满足同等安全语义的 rsync；无法保证时回退 SFTP。显式 `--method rsync` 无法保证时直接失败。

仅在明确需要时使用：

```bash
--no-validate-checksum
```

跳过 checksum 后，已有目标无法判断是否相同，仍需 `--overwrite` 或 `--backup`。

## Deploy

Deploy 使用模块化 playbook：文档 `version: 3`，由 plays（工作流）、tasks（任务）和 modules（模块）组成。文件结构沿用 `deploy.yaml` + `deploy.d/*.yaml`。

```bash
sshmd deploy validate
sshmd deploy list
sshmd deploy plan update-app
sshmd deploy run update-app --check --diff --yes
```

支持 13 个幂等模块：`command`/`shell`、`file`、`copy`、`template`、`service`、`wait_for`、`sleep`、`unarchive`、`fetch`、`pause`、`fail`、`debug`。每个模块内置 check/diff 与 changed 判定，另支持 `register`/`when`、`loop`、`run_once`、`ignore_errors`、`failed_when`/`changed_when`、`become`、`confirm`（linear 策略下每个 serial 批次开始前的人工门禁）、`block`/`rescue`/`always`、静态 `include`、`strategy: linear|free` 与 `gather_facts`。

### when 条件语法

`when` 使用小型的布尔表达式语言，支持：

```text
比较：    ==  !=  <  <=  >  >=
逻辑：    &&  ||  !  以及单词 and or not
成员：    item in list / string    item not in list / string
存在：    var is defined           var is not defined
字面量：  数字、'字符串'、true、false、null
路径：    upload.changed、item、facts.hostname
括号：    (expr)
```

两点行为约定：

- 引用未定义变量是错误，而不是静默跳过——任务不会因为拼写错误而悄悄不执行；可选变量请先写 `var is defined and ...`（`&&`/`||` 会短路，右侧不会误报）。
- 列表/字典可以比较相等或不等，但不能排序比较（`<`/`>` 会报错）；空列表与空字典在条件中为假。
- 普通任务 `when` 求值一次；loop 任务按每个 item 求值，可用 `when: item != 'x'` 跳过单项。loop 的 register 结果是列表，`when` 不支持 `results[n]` 索引。

### command 与 shell 模块

- `command` 不经过远程 shell：`cmd` 不能包含管道、重定向、`$`、`;`、`&`、反引号等 shell 元字符，参数按字面传递。
- 需要管道/重定向/变量展开时使用 `shell` 模块。
- `--check` 模式默认跳过 command/shell（只读检查不执行命令）；确需在 check 下执行的只读命令（如 `nginx -t`、`systemctl status`）给任务加 `check_safe: true`。

### 变量插值

`{{ }}` 插值支持 `vars`、play 级 `vars`、`vars_files` 与 CLI `--extra-var`，白名单函数：`default`、`join`、`upper`、`lower`、`trim`、`replace`、`shellquote`。缺失变量默认报错；`{{ missing | default "fallback" }}` 提供默认值。

## 配置与安全

- 默认主机信任策略是 `strict`。
- 初始化生成的主配置包含字段级中文说明；sshmd 保存配置时会恢复官方注释，自定义说明应写入主机或标签的 `note` 字段。
- 可以通过菜单/命令或手工编辑 `sshmd.yaml` 添加主机；手工新增的 `hosts` 条目可省略内部 `id`，sshmd 会自动生成并写回。已有主机的 `id` 用于关联凭据，不应修改。
- `--yes` 只跳过当前操作确认，不跳过主密码或 host trust。
- `--all` 不能与具体主机或 `--tag` 混用，避免意外扩大操作范围。
- `passwd` 和 `forget-pass` 支持多个主机、`--tag` 与 `--all`；批量 `passwd` 会把同一个 SSH 密码保存到全部目标主机。
- 删除保存密码、删除托管密钥、清理日志和设置 `host-key-policy insecure` 默认需要确认；非交互环境必须显式使用 `--yes`。
- 密码与托管私钥默认保存在 `sshmd.yaml` 的加密 vault 中；也支持在主机条目显式写 `password` 明文字段（与 `password_ref` 互斥，受 0600 权限保护，`sshmd doctor` 会给出提醒）。Deploy 编排文件始终禁止保存密码或私钥。
- 主密码只在当前进程内按需解锁，`lock` 或进程退出后失效。
- host alias 采用跨平台安全字符规则，可直接用于多主机 pull 目录。
- 主配置和 Deploy 配置均严格拒绝未知字段。

## 日志

日志默认写入 `~/.sshmd/logs`，目录权限为 `0700`，文件权限为 `0600`。可在主配置中关闭日志或调整保留时间：

```yaml
defaults:
  logs:
    enabled: true
    retention: 30d
```

```bash
sshmd logs
sshmd logs clean
```

## 开发与验收

```bash
go test ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...
go build ./...
go test -race ./...
go list -m
```

设计决策见 [docs/adr](docs/adr)，当前产品设计见 [docs/PRODUCT_DESIGN.md](docs/PRODUCT_DESIGN.md)，后续范围见 [ROADMAP.md](ROADMAP.md)。

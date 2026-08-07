# sshm v6.1.0

sshm 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。它使用 Go 原生 SSH 能力管理主机、标签、凭据、批量命令、安全文件传输和 Deploy v3 编排。

> 版本说明：产品发布版本是 `v6.1.0`，Go module 是 `github.com/fanhuadesenlinnn/sshm/v6`，主配置 schema 为 `version: 2`，Deploy 配置 schema 为 `version: 3`。

## 安装

### 一键安装

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/fanhuadesenlinnn/sshm/main/scripts/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/fanhuadesenlinnn/sshm/main/scripts/install.ps1 | iex
```

安装器会自动识别操作系统和 AMD64/ARM64 架构，从最新 GitHub Release 下载对应制品，使用 `checksums.txt` 验证 SHA-256，然后安装并执行 `sshm --version`。安装脚本只保存在代码仓库中，不会作为 Release 附件发布。

macOS/Linux 默认安装到 `/usr/local/bin`，权限不足时会请求 `sudo`；也可以指定版本或安装目录：

```bash
curl -fsSL https://raw.githubusercontent.com/fanhuadesenlinnn/sshm/main/scripts/install.sh | \
  sh -s -- --version v6.1.0 --install-dir "$HOME/.local/bin"
```

Windows 默认安装到 `%LOCALAPPDATA%\Programs\sshm` 并加入用户 PATH。

### 使用 Go 安装

已经安装 Go 1.25 或更高版本时，也可以使用：

```bash
go install github.com/fanhuadesenlinnn/sshm/v6@latest
```

如果 `proxy.golang.org` 访问较慢，可临时指定国内代理：

```bash
GOPROXY=https://goproxy.cn,direct go install github.com/fanhuadesenlinnn/sshm/v6@latest
```

PowerShell：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"; go install github.com/fanhuadesenlinnn/sshm/v6@latest
```

也可以前往 [GitHub Releases](https://github.com/fanhuadesenlinnn/sshm/releases/latest) 手工下载对应平台的压缩包和校验文件。

## v6 破坏性变更

- 默认数据目录只使用 `~/.sshm`，唯一可用的路径覆盖变量是 `SSHM_HOME`。
- 不支持 `SSHM_CONFIG_FILE`，也不读取、迁移或删除旧 `~/.config/sshm/sshm.yaml`。
- 主配置必须显式使用 `version: 2`，不兼容旧 schema。
- Deploy v2（`version: 2` 的 profile/steps/handlers）在 v6.1.0 移除，`deploy migrate` 一并移除；旧文件不会被加载，请改写为 v3 playbook。
- `exec-all`、`push-all`、`pull-all` 已移除，统一使用虚拟标签 `all`。
- 当前目录的 `sshm.deploy.yaml` 不再隐式加载，项目文件必须通过 `--file` 指定。

## 首次使用

```bash
sshm
sshm init
sshm config path
sshm doctor
```

未初始化时直接运行 `sshm` 会显示首次使用引导，不会直接进入空工作台。

`sshm init` 创建：

```text
~/.sshm/
├── sshm.yaml
├── deploy.yaml
├── deploy.d/
├── templates/
├── README.md
├── logs/
├── backups/
└── tmp/
```

`sshm.yaml` 会包含快速开始、字段用途、主机示例和安全边界说明。`deploy.yaml` 默认不启用任何工作流，提供一份完全注释掉的 Deploy v3 示例（快速开始、全部模块、register/when、block/rescue、include、sleep/confirm）；可以安全地先运行 `sshm deploy validate`，再按注释创建 play。`templates/` 含一个可运行的模板示例，`README.md` 是一页速查。已有 `deploy.yaml` 不会被 `sshm init --force` 覆盖。

主配置、日志、deploy 文件和备份都以同一个 `SSHM_HOME` 为根目录。发现旧配置时，`init` 与 `doctor` 只输出警告。

## 常用命令

```bash
sshm                         # 打开轻量工作台
sshm web01                   # 按别名或 ID 直连
sshm list
sshm add web01 root@10.0.0.11
sshm edit web01
sshm tag
sshm ping web01
sshm passwd web01 web02
sshm passwd --tag prod
sshm exec web01 "uptime"
sshm exec-tag prod "uptime"
sshm exec-tag all "uptime"
```

Cobra 提供根命令和 Deploy 子命令帮助：

```bash
sshm --help
sshm host --help
sshm key --help
sshm tag --help
sshm deploy --help
sshm deploy run --help
```

在交互工作台中，复杂远程命令建议用 `--` 明确标记起点；`--` 后的文本不再由 sshm 拆分或重组：

```text
sshm> x web01 --quiet -- pwd
sshm> x web01 -- awk '{print $1}' /tmp/data
sshm> xt prod --parallel 4 --yes -- systemctl restart app
```

不写 `--` 时仍兼容 `x web01 pwd` 和 `x web01 --quiet pwd`。只有远程命令开始前的已知 sshm 选项会被解析；一旦识别到命令起点，后续引号、变量、反斜杠和命令参数都原样传给远程 Shell。

## 批量执行

`exec-tag`、`push-tag`、`pull-tag` 和 `deploy run` 复用同一个 BatchRunner。

```bash
sshm exec-tag prod "systemctl status app" \
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
sshm push web01 ./dist/app.tar.gz /opt/app/app.tar.gz
sshm pull web01 /etc/nginx/nginx.conf ./nginx.web01.conf
```

按标签：

```bash
sshm push-tag prod ./dist/app.tar.gz /opt/app/app.tar.gz --backup --yes
sshm pull-tag prod /etc/nginx ./backup --yes
sshm pull-tag all /etc/hosts ./backup --flat --yes
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

Deploy 使用模块化 playbook：文档 `version: 3`，由 plays（工作流）、tasks（任务）和 modules（模块）组成。文件结构沿用 `deploy.yaml` + `deploy.d/*.yaml`；v6.1.0 起仅支持 v3，不再加载 Deploy v2 文件。

```bash
sshm deploy validate
sshm deploy list
sshm deploy plan update-app
sshm deploy run update-app --check --diff --yes
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

### 变量插值

`{{ }}` 插值支持 `vars`、play 级 `vars`、`vars_files` 与 CLI `--extra-var`，白名单函数：`default`、`join`、`upper`、`lower`、`trim`、`replace`、`shellquote`。缺失变量默认报错；`{{ missing | default "fallback" }}` 提供默认值。

### 升级注意（v6.1.0）

- Deploy v2（`version: 2` 的 profile/steps/handlers）与 `deploy migrate` 已移除；旧文件不会被加载，请改写为 v3 playbook。
- v2 的 `wait`（定时延时）由 `sleep` 模块替代；v2 的 `confirm` 批次门禁由任务级 `confirm` 字段替代。
- 主配置 schema 仍为 `version: 2`，不受影响。

## 配置与安全

- 默认主机信任策略是 `strict`。
- 初始化生成的主配置包含字段级中文说明；sshm 保存配置时会恢复官方注释，自定义说明应写入主机或标签的 `note` 字段。
- 可以通过菜单/命令或手工编辑 `sshm.yaml` 添加主机；手工新增的 `hosts` 条目可省略内部 `id`，sshm 会自动生成并写回。已有主机的 `id` 用于关联凭据，不应修改。
- `--yes` 只跳过当前操作确认，不跳过主密码或 host trust。
- `--all` 不能与具体主机或 `--tag` 混用，避免意外扩大操作范围。
- `passwd` 和 `forget-pass` 支持多个主机、`--tag` 与 `--all`；批量 `passwd` 会把同一个 SSH 密码保存到全部目标主机。
- 删除保存密码、删除托管密钥、清理日志和设置 `host-key-policy insecure` 默认需要确认；非交互环境必须显式使用 `--yes`。
- 密码与托管私钥保存在 `sshm.yaml` 的加密 vault 中。
- 主密码只在当前进程内按需解锁，`lock` 或进程退出后失效。
- host alias 采用跨平台安全字符规则，可直接用于多主机 pull 目录。
- 主配置和 Deploy 配置均严格拒绝未知字段。

## 日志

日志默认写入 `~/.sshm/logs`，目录权限为 `0700`，文件权限为 `0600`。可在主配置中关闭日志或调整保留时间：

```yaml
defaults:
  logs:
    enabled: true
    retention: 30d
```

```bash
sshm logs
sshm logs clean
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

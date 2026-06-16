# sshm v6.0.1

sshm 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。它使用 Go 原生 SSH 能力管理主机、标签、凭据、批量命令、安全文件传输和 Deploy v2 编排。

> 版本说明：产品发布版本是 `v6.0.1`，Go module 是 `github.com/fanhuadesenlinnn/sshm/v6`，主配置与 Deploy 配置的 schema 都是 `version: 2`。

## 安装

需要 Go 1.25 或直接下载 GitHub Release 中对应平台的二进制。

```bash
go install github.com/fanhuadesenlinnn/sshm/v6@v6.0.1
```

## v6 破坏性变更

- 默认数据目录只使用 `~/.sshm`，唯一可用的路径覆盖变量是 `SSHM_HOME`。
- 不支持 `SSHM_CONFIG_FILE`，也不读取、迁移或删除旧 `~/.config/sshm/sshm.yaml`。
- 主配置必须显式使用 `version: 2`，不兼容旧 schema。
- Deploy 必须显式使用嵌套 action DSL `version: 2`，不兼容 Deploy v1。
- `exec-all`、`push-all`、`pull-all` 已移除，统一使用虚拟标签 `all`。
- 当前目录的 `sshm.deploy.yaml` 不再隐式加载，项目文件必须通过 `--file` 指定。

## 首次使用

```bash
sshm init
sshm config path
sshm doctor
```

`sshm init` 创建：

```text
~/.sshm/
├── sshm.yaml
├── deploy.d/
├── logs/
├── backups/
└── tmp/
```

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
sshm exec web01 "uptime"
sshm exec-tag prod "uptime"
sshm exec-tag all "uptime"
```

Cobra 提供根命令和 Deploy 子命令帮助：

```bash
sshm --help
sshm deploy --help
sshm deploy run --help
```

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
--yes
--no-log
--quiet
```

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
- 拒绝符号链接、设备文件、socket、FIFO 等特殊文件。
- 多主机 pull 默认保存为 `local/host_alias/remote-path`。
- `--flat` 会在执行前检查本次操作内部的目标冲突。
- `auto` 可使用满足同等安全语义的 rsync；无法保证时回退 SFTP。显式 `--method rsync` 无法保证时直接失败。

仅在明确需要时使用：

```bash
--no-validate-checksum
```

跳过 checksum 后，已有目标无法判断是否相同，仍需 `--overwrite` 或 `--backup`。

## Deploy v2

默认加载存在的 `~/.sshm/deploy.yaml` 与按文件名排序的 `~/.sshm/deploy.d/*.yaml`。显式 `--file` 时只加载指定文件。

```bash
sshm deploy init
sshm deploy validate
sshm deploy list
sshm deploy plan update-app
sshm deploy run update-app --check --diff --yes
sshm deploy run update-app --serial 2 --parallel 2 --yes
```

示例：

```yaml
version: 2

profiles:
  - name: update-nginx
    targets:
      tags: [web]
    serial: 2
    parallel: 2
    steps:
      - name: upload config
        push:
          src: ./nginx.conf
          dest: /etc/nginx/nginx.conf
          backup: true
        notify: [reload nginx]

      - name: test config
        exec: nginx -t
        become: true

handlers:
  - name: reload nginx
    exec: systemctl reload nginx
    become: true
```

一个 step 或 handler 必须且只能包含一个 action：`exec`、`push`、`pull`、`mkdir`、`wait`、`confirm`。handler 不允许 `confirm` 或继续 `notify`。

Deploy v2 支持：

- 静态 `plan`，不会连接远端。
- `--check` 区分 `would-change`、`ok` 与 `skipped`，不修改最终目标。
- `--diff` 展示文本 push 差异；diff 默认不写入日志。
- `notify` / handlers、`ignore_error`、`failed_when`、`changed_when`。
- `become: true` 使用 `sudo -n -u <user> -- sh -c '<安全引用命令>'`。
- `serial`、`parallel` 与失败策略复用共享 BatchRunner。

## 配置与安全

- 默认主机信任策略是 `strict`。
- `--yes` 只跳过当前操作确认，不跳过主密码或 host trust。
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
go build ./...
go test -race ./...
go list -m
```

设计决策见 [docs/adr](docs/adr)，当前产品设计见 [docs/PRODUCT_DESIGN.md](docs/PRODUCT_DESIGN.md)，后续范围见 [ROADMAP.md](ROADMAP.md)。

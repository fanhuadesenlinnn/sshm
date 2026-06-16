# sshm v6.0.1 产品设计

状态：v6.0.1 当前设计

## 产品定位

sshm 是供个人开发者和个人运维使用者管理约 5 到 100 台 SSH 主机的本地工具。它提供比手写 OpenSSH 配置更清晰的主机组织、凭据管理、批量操作、安全传输和轻量部署能力。

明确非目标：

- 团队空间、权限审批、集中审计和共享凭据。
- 后台调度、守护进程和持续部署服务。
- 完整 Ansible 兼容层、roles、facts、循环或期望状态收敛。
- 任意 OpenSSH 参数透传。

## 当前产品事实

- 产品版本为 `v6.0.1`，Go module 为 `/v6`。
- 主配置与 Deploy 配置均为严格 YAML schema `version: 2`。
- 默认数据目录只使用 `~/.sshm`，仅支持 `SSHM_HOME` 整体覆盖。
- Cobra 提供 CLI 命令树，同时保留无参数工作台和 alias/ID 直连。
- Go 原生 SSH 负责连接、认证、host trust、执行、SFTP 和端口转发。
- rsync 是可选加速路径，必须保持与 SFTP 相同的安全和结果语义。
- Linux、macOS、Windows 提供相同正式能力目标。

## 核心用户旅程

### 初始化与发现

新用户运行 `sshm init` 创建完整 v2 主配置和受限权限目录。发现旧 `~/.config/sshm/sshm.yaml` 时只警告，不读取、迁移或删除。

用户可通过工作台、搜索、收藏、最近使用、标签和 `sshm <alias|ID>` 快速定位主机。

### 安全认证

主机默认使用 strict host trust。密码和托管密钥由主密码保护并保存在主配置的加密 vault 中。主密码只在当前进程按需解锁。

`--yes` 只跳过一次普通操作确认，不跳过主密码或 host trust。

### 批量操作

`exec-tag`、`push-tag`、`pull-tag` 和 `deploy run` 使用共享 BatchRunner：

- 稳定目标顺序。
- serial 批次和批内 parallel。
- fail-fast、max-fail、max-fail-percent。
- Ctrl+C 取消与未开始任务 skipped。
- 统一结果状态与退出码。

`all` 是虚拟标签，统一替代旧 `*-all` 命令。

### 文件传输

push/pull 支持文件和目录。默认 SHA-256 校验，目录使用逐文件 manifest。目标不同且已存在时默认拒绝，用户必须显式选择 `--overwrite` 或 `--backup`。

传输始终先写临时目标，校验后 rename。符号链接、设备文件、socket、FIFO 等特殊文件被拒绝。

多主机 pull 默认使用 `local/host_alias/remote-path`；`--flat` 在任何下载开始前检测冲突。

### Deploy v2

Deploy v2 是轻量 Ansible 风格执行模型，不是通用工作流语言。

- 默认加载用户数据目录中的 `deploy.yaml` 与排序后的 `deploy.d/*.yaml`。
- 显式 `--file` 时只加载指定文件。
- 不隐式发现当前目录文件。
- 一个 step 或 handler 必须且只能包含一个嵌套 action。
- 支持 exec、push、pull、mkdir、wait、confirm。
- 支持 plan、check、diff、changed、notify/handlers、ignore_error、简单 rc conditions 和 become。
- serial、parallel 和失败策略复用共享 BatchRunner。

## 安全原则

1. 默认严格，便捷选项必须显式。
2. 失败不能静默降低 host trust、凭据保护、覆盖策略或路径安全。
3. 批量写入和命令执行默认展示具体目标并确认。
4. 配置、传输和 Deploy 在执行前尽可能完成静态校验。
5. 原文件或最终目标在失败时保持可用。
6. 日志权限仅限当前用户，且 Deploy diff 默认不进入日志。

## 配置与路径

```text
<SSHM_HOME>/
├── sshm.yaml
├── deploy.yaml
├── deploy.d/
├── logs/
├── backups/
└── tmp/
```

主配置是唯一权威可变状态。Deploy 文件是只读声明输入，不保存 SSH 用户、密码、私钥、端口或 host trust。

## 输出与退出码

状态：

```text
ok changed would-change failed unreachable skipped
```

退出码：

```text
0 success
1 failed 或失败策略产生 skipped
2 unreachable
3 参数或配置错误
4 执行前 vault/auth 阻断
130 用户中断
```

## 发布质量

发布前必须通过：

```bash
go test ./...
go vet ./...
go build ./...
go test -race ./...
go list -m
```

当前设计决策记录在 `docs/adr/`，后续范围记录在根目录 `ROADMAP.md`。

# sshm

`sshm` 是面向个人使用者的本地优先 SSH 工作台，适合管理约 5～100 台主机。

它提供主机与标签管理、收藏与最近使用、加密凭据、托管密钥、批量命令、安全文件传输、跳板机和本地端口转发。

## 安装

要求 Go 1.25 或更高版本。

```bash
go install github.com/fanhuadesenlinnn/sshm/v4@latest
```

也可以从源码构建：

```bash
git clone https://github.com/fanhuadesenlinnn/sshm.git
cd sshm
go build -o sshm .
```

## 快速开始

```bash
# 添加纯本地主机配置
sshm add prod root@example.com --tags linux,prod

# 保存 SSH 密码，或创建并绑定托管密钥
sshm passwd prod
sshm key create personal --default
sshm key setup personal prod

# 连接、诊断和操作
sshm prod
sshm ping prod
sshm exec prod "uname -a"
sshm push ./app.tar.gz /tmp/app.tar.gz prod
sshm pull /var/log/app.log ./downloads prod
```

无参数启动进入轻量工作台：

```bash
sshm
```

## 单文件配置

所有配置、主机信任、加密密码和加密私钥均保存在：

```text
~/.config/sshm/sshm.yaml
```

可通过 `SSHM_HOME` 修改数据目录，或通过 `SSHM_CONFIG_FILE` 指定配置文件。

程序使用临时文件、`fsync` 和原子替换更新配置，不生成 `.bak`。操作日志保存在同一数据目录的 `logs/` 中。

首次使用时会生成带简短编辑提示的配置：

```yaml
# sshm 配置；完整说明: sshm help config
# host_key_policy: strict | accept-new | insecure(跳过验证)；主机空值继承全局
# 主机必填: alias, user, host；常用可选: port, auth, identity, tags
# auth: auto | key | password；identity 填托管密钥名
# 高级可选: host_key_policy, jump_host
# host_trust 与 vault 由 sshm 管理，请勿手动修改
version: 1
defaults:
  host_key_policy: strict
hosts: []
managed_keys:
  items: []
host_trust:
  entries: []
```

v4 不读取或迁移旧版 `hosts.yaml`、`keys.yaml`、`secrets.yaml` 与 `.bak`；这些文件会被忽略。

完整编辑配置：

```bash
sshm config-edit
```

设置全局主机信任策略：

```bash
sshm config host-key-policy strict
sshm config host-key-policy accept-new
sshm config host-key-policy insecure
```

主机可以通过 `host_key_policy` 覆盖全局设置。有效顺序为：

```text
主机覆盖 > 全局默认 > strict
```

- `strict`：未知主机需要交互确认，密钥变化时拒绝连接。
- `accept-new`：自动记录未知主机，密钥变化时拒绝连接。
- `insecure`：显式跳过主机身份验证，连接时显示警告。

## 常用命令

### 主机

```bash
sshm list [--tag 标签] [--sort id|alias]
sshm show <别名|ID>
sshm search <关键词...>
sshm tag [标签]
sshm pin <别名|ID>
sshm recent
sshm edit <别名|ID>
sshm delete <别名|ID>
sshm pick
```

快速添加支持以下选项：

```bash
sshm add <别名> <用户@主机[:端口]> \
  [--tags 标签] [--note 备注] [--auth auto|key|password] \
  [--identity 托管密钥] [--host-key-policy 策略] [--jump-host 别名]
```

添加主机不会连接远端、验证认证或修改远端。

### 凭据与托管密钥

```bash
sshm passwd <主机>
sshm forget-pass <主机>
sshm lock

sshm key list
sshm key create <名称> [--default]
sshm key import <名称> <私钥路径> [--default]
sshm key default [名称|-]
sshm key show [名称|default]
sshm key use <密钥> <目标...>
sshm key push <密钥> <目标...> [--yes] [--quiet]
sshm key setup <密钥> <目标...> [--yes] [--quiet]
sshm key revoke <密钥> <目标...> [--yes] [--quiet]
sshm key delete <名称...>
```

普通私钥文件仅作为显式导入来源。后续连接只使用由主密码保护的托管密钥。

目标支持主机别名、`--tag 标签` 和 `--all`。

### 远程操作

```bash
sshm ping [--yes] [--quiet] [主机]

sshm exec [--yes] [--quiet] <主机> <命令>
sshm exec-tag [--yes] [--quiet] <标签> <命令>
sshm exec-all [--yes] [--quiet] <命令>
```

远程命令和所有多主机操作默认需要确认。非交互环境必须显式使用 `--yes`。

批量操作会展示逐主机失败阶段、原因、恢复建议和单主机重试命令。`--quiet` 隐藏成功输出，完整逐主机结果写入操作日志：

```bash
sshm logs
sshm logs clean
```

### 文件传输

文件传输支持文件和目录。直连托管密钥主机在本地与远端均可用 `rsync` 时优先安全加速；其他情况自动使用原生 SFTP。两条路径共享相同的主机信任、覆盖和临时目标语义。

```bash
sshm push <本地路径> <远程路径> <目标...> [--overwrite] [--yes] [--quiet]
sshm pull <远程路径> <本地目录> <目标...> [--overwrite] [--yes] [--quiet]
```

- 默认拒绝覆盖。
- 拉取结果按主机别名分目录保存。
- 内容先写入临时目标，成功后再启用最终目标。
- 所有传输使用与连接、执行相同的认证和主机信任策略。
- `rsync` 加速使用一次性私钥与专用 `known_hosts`，失败后清理并回退 SFTP。

### 跳板机与端口转发

主机通过 `jump_host` 引用另一台已配置主机。仅支持单级跳板机，两段连接分别验证主机身份和认证。

```bash
sshm forward <主机> <本地监听地址> <远程目标>
sshm forward prod 127.0.0.1:8080 127.0.0.1:80
```

按 `Ctrl+C` 关闭转发。

### OpenSSH 配置

```bash
sshm import-ssh-config [输入文件]
sshm export-ssh-config <输出文件>
```

导入时，引用 `IdentityFile` 的主机会被明确跳过。请先通过 `sshm key import` 导入私钥。

## 安全边界

- 主密码只在当前进程内解锁，`lock` 或进程退出后清除。
- 保存密码和托管私钥在 `sshm.yaml` 的加密 vault 中。
- 所有 SSH 能力共享同一认证与主机信任实现。
- 不透传任意 OpenSSH 参数，不静默降低安全策略。
- 系统 `ssh` 仅可作为受约束的可选 `rsync` 传输通道，不用于连接或失败回退。
- `--yes` 只跳过当前操作确认，不跳过主密码或主机信任。
- `insecure` 必须由全局或单主机配置显式启用。

## 平台

正式发布构建：

- Linux amd64 / arm64
- macOS amd64 / arm64
- Windows amd64 / arm64

Linux 制品使用纯 Go 静态构建。

## 开发验证

```bash
go test ./...
go test -race ./...
go vet ./...
```

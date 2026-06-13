# sshm - SSH 主机管理器

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)

`sshm` 是一个轻量级的命令行 SSH 主机管理工具，提供交互式终端界面和命令行两种使用方式，帮助你快速管理、连接和操作多台远程服务器。

## ✨ 功能特性

- **主机管理** — 添加、编辑、删除、搜索、查看 SSH 主机配置
- **分组管理** — 按分组组织主机，支持分组批量执行命令
- **命令工作台** — 无参数启动直接进入带快捷提示的命令页，`p` 随时打开主机选择器
- **收藏与最近连接** — 常用主机置顶，快速回到最近使用的服务器
- **现代命令语法** — 支持 `sshm add/list/edit`，并兼容旧版 `--add/--list`
- **Shell 自动补全** — 支持 bash、zsh、fish 的命令与主机别名补全
- **快速连接** — 通过别名或 ID 一键连接远程主机
- **批量执行** — 支持单机、分组、全部主机远程执行命令
- **连通性测试** — `ping` 命令快速检测主机可达性
- **SSH 配置导入/导出** — 兼容 OpenSSH `~/.ssh/config` 格式
- **密码管理** — 加密存储 SSH 密码，主密码保护
- **托管密钥** — 自定义命名、默认密钥、批量创建/导入/绑定/删除
- **主密码保护私钥** — 托管私钥只保存在加密密码库中，普通连接在内存中使用
- **公钥部署** — 单机或按分组/标签批量推送、验证、撤销公钥
- **批量主机管理** — 交互式批量添加，或使用 `$EDITOR` 校验后直接编辑配置
- **认证策略** — 灵活配置密码/密钥/交互式认证
- **跨平台** — 支持 Linux、macOS、Windows；Linux 发布包为静态二进制，不依赖 glibc

## 📦 安装

### 从源码构建

```bash
git clone https://github.com/fanhuadesenlinnn/sshm.git
cd sshm
go build -o sshm .
```

然后将编译好的 `sshm` 二进制文件移动到 `PATH` 中的目录：

```bash
# Linux / macOS
sudo mv sshm /usr/local/bin/

# 或仅当前用户
mkdir -p ~/bin && mv sshm ~/bin/
```

### Go 安装

```bash
go install github.com/fanhuadesenlinnn/sshm@latest
```

### Linux 兼容性

正式发布的 Linux amd64 和 arm64 制品使用纯 Go 静态构建，不依赖目标机器的 glibc，可用于常见的旧版 Linux、Alpine Linux 和精简容器环境。

发布流程会在 CentOS 7（glibc 2.17）和 Alpine Linux（musl）中实际启动 amd64 制品。极旧 Linux 内核仍需满足当前 Go 运行时要求。

如果之前遇到以下错误，请升级到 v2.3.1 或更高版本：

```text
GLIBC_x.xx not found
/lib64/ld-linux-x86-64.so.2: No such file or directory
```

## 🚀 快速开始

### 进入命令工作台

```bash
sshm
```

启动后会显示与 `h` 相同的快捷命令提示：

```
sshm> a                 # 添加主机
sshm> ab                # 批量添加主机
sshm> p                 # 打开可搜索主机选择器
sshm> k                 # 进入托管密钥中心
sshm> host              # 进入主机管理中心
```

### 推荐的托管密钥流程

```bash
# 1. 创建自己的默认密钥；私钥由 sshm 主密码加密
sshm key create personal --default

# 2. 批量添加主机，交互模式下可选择逐台保存 SSH 密码
sshm add-batch web-1=root@10.0.0.11 web-2=root@10.0.0.12

# 3. 向分组、标签或全部主机推送公钥，验证成功后再绑定
sshm key setup default --group prod
sshm key setup default --tag linux
sshm key setup default --all

# 4. 后续直接通过 sshm 连接；私钥不会以明文常驻磁盘
sshm web-1
```

`key push` 只推送公钥，`key use` 只修改本地绑定，`key setup` 会依次执行推送、验证和绑定。批量操作默认 4 并发，并逐台输出结果。

### 添加主机

```bash
# 推荐：直接快速添加
sshm add prod root@10.0.0.10
sshm add prod-web deploy@example.com:2222 --group prod --tags web,linux
sshm add bastion root@bastion.example.com --identity ~/.ssh/id_ed25519

# 无参数时使用添加向导
sshm add
```

快速添加默认使用 `auto` 认证，不会询问密码或高级选项。需要保存密码时，可在添加后执行 `sshm passwd <别名>`。

无参数添加向导在未填写密钥时会主动进入密码设置流程。批量添加在交互终端中也可选择逐台保存密码。

### 连接主机

```bash
# 通过别名连接
sshm my-server

# 通过 ID 连接
sshm 1

# 传递额外的 SSH 参数
sshm my-server -L 8080:localhost:8080
```

### 列出所有主机

```bash
sshm list
sshm list --group prod --sort alias
sshm list --wide
```

### 搜索、收藏与最近使用

```bash
sshm search web prod
sshm search group:prod tag:nginx
sshm pin prod-web
sshm recent
sshm unpin prod-web
```

### Shell 自动补全

```bash
# zsh
mkdir -p ~/.zfunc
sshm completion zsh > ~/.zfunc/_sshm
fpath=(~/.zfunc $fpath)
autoload -Uz compinit && compinit

# bash
mkdir -p ~/.local/share/bash-completion/completions
sshm completion bash > ~/.local/share/bash-completion/completions/sshm

# fish
mkdir -p ~/.config/fish/completions
sshm completion fish > ~/.config/fish/completions/sshm.fish
```

## 📖 命令参考

### 主机管理

| 命令 | 说明 |
|------|------|
| `sshm` | 进入交互模式 |
| `sshm host` | 进入主机管理中心 |
| `sshm key` | 进入托管密钥中心 |
| `sshm <别名\|ID>` | 连接到指定主机 |
| `sshm connect <别名\|ID>` | 显式连接主机，适用于别名与命令同名时 |
| `sshm pick` | 打开实时主机选择器 |
| `sshm list [--group 分组] [--sort id\|alias\|group]` | 列出、过滤和排序主机 |
| `sshm add [别名 用户@主机[:端口]]` | 快速添加或进入添加向导 |
| `sshm add-batch [别名=用户@主机[:端口] ...]` | 批量添加主机 |
| `sshm edit <别名\|ID>` | 编辑主机配置，可输入 `-` 清空可选字段 |
| `sshm delete <别名\|ID>` | 删除主机 |
| `sshm show <别名\|ID>` | 显示主机详情 |
| `sshm search <关键词...>` | 多关键词搜索，支持 `group:` 和 `tag:` |
| `sshm group [分组名]` | 列出分组或分组内的主机 |
| `sshm copy <别名\|ID>` | 复制包含密钥参数的 SSH 连接命令 |
| `sshm pin/unpin <别名\|ID>` | 收藏或取消收藏主机 |
| `sshm recent [数量]` | 显示收藏与最近连接 |
| `sshm config-edit` | 使用 `$EDITOR` 编辑，校验通过后原子更新配置 |

### 连接与执行

| 命令 | 说明 |
|------|------|
| `sshm ping [别名\|ID]` | 测试主机连通性 |
| `sshm exec <目标> <命令>` | 在指定主机上执行命令 |
| `sshm exec-group <分组> <命令>` | 在分组内所有主机执行命令 |
| `sshm exec-all <命令>` | 在所有主机上执行命令 |

### 认证与安全

| 命令 | 说明 |
|------|------|
| `sshm passwd <别名\|ID>` | 设置 SSH 密码（加密存储） |
| `sshm forget-pass <别名\|ID>` | 删除已存储的密码 |
| `sshm import-key <别名\|ID> <路径>` | 导入 SSH 私钥 |
| `sshm gen-key <别名\|ID>` | 生成新的 SSH 密钥对 |
| `sshm show-pubkey <别名\|ID>` | 显示公钥内容 |
| `sshm auth <别名\|ID>` | 修改认证策略 |
| `sshm lock` | 锁定当前会话已解锁的密码库 |

旧版 `import-key/gen-key/show-pubkey` 命令继续兼容。新项目推荐使用以下托管密钥命令：

| 命令 | 说明 |
|------|------|
| `sshm key list` | 列出托管密钥、默认项和指纹 |
| `sshm key create <名称> [--default]` | 在内存生成并加密保存 Ed25519 密钥 |
| `sshm key create-batch <名称...>` | 批量生成托管密钥 |
| `sshm key import <名称> <路径> [--default]` | 导入私钥，去除原密码后由 sshm 主密码保护 |
| `sshm key import-batch <名称=路径...>` | 批量导入私钥 |
| `sshm key default [名称\|-]` | 查看、设置或取消默认密钥 |
| `sshm key show [名称\|default]` | 显示托管公钥 |
| `sshm key use <密钥> <目标...>` | 修改本地主机密钥绑定 |
| `sshm key push <密钥> <目标...>` | 幂等推送公钥到远端 |
| `sshm key setup <密钥> <目标...>` | 推送、验证后绑定 |
| `sshm key revoke <密钥> <目标...>` | 从远端精确撤销公钥 |
| `sshm key status [密钥]` | 查看主机与密钥绑定 |
| `sshm key delete <名称...>` | 批量删除本地托管密钥 |
| `sshm key delete-unused` | 删除未绑定且非默认的托管密钥 |

目标选择器支持主机别名、`--group 分组`、`--tag 标签` 和 `--all`。

### 配置管理

| 命令 | 说明 |
|------|------|
| `sshm export-ssh-config [文件]` | 导出为 OpenSSH 配置格式 |
| `sshm import-ssh-config [文件]` | 从 OpenSSH 配置导入主机 |
| `sshm doctor` | 检查配置路径、系统 SSH 和密钥文件 |
| `sshm completion <bash\|zsh\|fish>` | 生成 Shell 自动补全脚本 |

### 全局选项

| 选项 | 说明 |
|------|------|
| `--help, -h` | 显示帮助信息 |
| `--version, -v` | 显示版本信息 |

所有旧版 `--list`、`--add`、`--edit` 等参数仍然兼容。

## 🛠 技术架构

```
sshm/
├── main.go                  # 入口
├── internal/
│   ├── command/             # CLI 命令处理
│   │   ├── root.go          # 命令路由 & 帮助
│   │   ├── interactive.go   # 交互模式
│   │   ├── favorites.go     # 收藏与最近使用
│   │   ├── completion.go    # Shell 自动补全
│   │   ├── doctor.go        # 环境检查
│   │   ├── add.go           # 添加主机
│   │   ├── edit.go          # 编辑主机
│   │   ├── delete.go        # 删除主机
│   │   ├── list.go          # 列出主机
│   │   ├── show.go          # 显示详情
│   │   ├── search.go        # 搜索主机
│   │   ├── copy.go          # 复制连接信息
│   │   ├── group.go         # 分组管理
│   │   ├── ping.go          # 连通性测试
│   │   ├── exec.go          # 远程执行
│   │   ├── key.go           # 密钥管理
│   │   ├── password.go      # 密码管理
│   │   └── ssh_config.go    # SSH 配置导入/导出
│   ├── config/              # 主机配置与存储
│   │   ├── host.go          # 主机数据结构
│   │   ├── store.go         # YAML 配置存储
│   │   ├── key_store.go     # 托管密钥元数据
│   │   └── paths.go         # 配置文件路径
│   ├── sshx/                # SSH 连接处理
│   │   ├── native.go        # Go 原生 SSH 客户端
│   │   ├── openssh.go       # OpenSSH 客户端转发
│   │   └── auth.go          # 认证策略
│   ├── secret/              # 加密存储
│   │   ├── crypto.go        # AES 加密/解密
│   │   └── file_store.go    # 加密文件存储
│   ├── keymgr/              # SSH 密钥管理
│   │   └── keymgr.go        # 密钥生成/导入
│   └── ui/                  # 终端 UI
│       ├── format.go        # 格式化输出
│       ├── table.go         # 表格渲染
│       ├── picker.go        # 实时主机选择器
│       └── prompt.go        # 交互提示
├── go.mod
├── go.sum
└── README.md
```

## 🔧 依赖

| 依赖 | 用途 |
|------|------|
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | SSH 协议实现 |
| [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) | 终端原始模式 |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | YAML 配置文件解析 |

## 🔐 安全与恢复

- 配置默认位于 `~/.config/sshm`，可通过 `SSHM_HOME` 修改。主机在 `hosts.yaml`，托管密钥元数据在 `keys.yaml`，密码与托管私钥在 `secrets.yaml`。
- v2.3.0 会自动把配置版本升级到 v3，用于记录收藏和最近连接时间，无需手动迁移。
- `hosts.yaml`、`secrets.yaml` 和导出的 SSH 配置在覆盖前保留最近一份 `.bak`。
- 密码库使用主密码加密；主密码不会保存且无法恢复。保存的 SSH 密码与托管私钥共用此保护，交互会话只需解锁一次，可用 `lock` 立即锁定。
- 托管私钥不会导出到 OpenSSH 配置。普通连接、执行和 ping 在内存中使用；仅在传递高级 OpenSSH 参数时创建会话临时私钥，并在 SSH 退出后删除。
- 原生密码连接严格校验 `~/.ssh/known_hosts`。首次连接仅在交互终端确认指纹后写入，非交互环境默认拒绝未知主机。
- 如果主配置损坏，请先退出 sshm，检查同目录下的 `.bak`，确认内容后再手动恢复，程序不会静默覆盖损坏文件。

## 📄 许可证

[MIT License](LICENSE)

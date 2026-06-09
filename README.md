# sshm - SSH 主机管理器

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)

`sshm` 是一个轻量级的命令行 SSH 主机管理工具，提供交互式终端界面和命令行两种使用方式，帮助你快速管理、连接和操作多台远程服务器。

## ✨ 功能特性

- **主机管理** — 添加、编辑、删除、搜索、查看 SSH 主机配置
- **分组管理** — 按分组组织主机，支持分组批量执行命令
- **交互模式** — 无参数启动进入交互式 Shell，支持自动补全和命令历史
- **快速连接** — 通过别名或 ID 一键连接远程主机
- **批量执行** — 支持单机、分组、全部主机远程执行命令
- **连通性测试** — `ping` 命令快速检测主机可达性
- **SSH 配置导入/导出** — 兼容 OpenSSH `~/.ssh/config` 格式
- **密码管理** — 加密存储 SSH 密码，主密码保护
- **密钥管理** — 导入、生成 SSH 密钥对，显示公钥
- **认证策略** — 灵活配置密码/密钥/交互式认证
- **跨平台** — 支持 Linux、macOS、Windows

## 📦 安装

### 从源码构建

```bash
git clone https://github.com/sshm/sshm.git
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
go install github.com/sshm/sshm@latest
```

## 🚀 快速开始

### 启动交互模式

```bash
sshm
```

进入交互式界面后，你可以使用以下命令：

```
sshm> add      # 添加新主机
sshm> list     # 列出所有主机
sshm> ping     # 测试连接
sshm> help     # 查看帮助
```

### 添加主机

```bash
# 命令行方式
sshm --add

# 交互模式下
sshm> add
```

按提示输入主机别名、地址、用户名、端口等信息。

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
sshm --list
```

## 📖 命令参考

### 主机管理

| 命令 | 说明 |
|------|------|
| `sshm` | 进入交互模式 |
| `sshm <别名\|ID>` | 连接到指定主机 |
| `sshm --list, -l` | 列出所有主机 |
| `sshm --add, -a` | 添加新主机 |
| `sshm --edit, -e <别名\|ID>` | 编辑主机配置 |
| `sshm --delete, -d <别名\|ID>` | 删除主机 |
| `sshm --show <别名\|ID>` | 显示主机详情 |
| `sshm --search, -s <关键词>` | 搜索主机 |
| `sshm --group, -g [分组名]` | 列出分组或分组内的主机 |
| `sshm --copy, --cp <别名\|ID>` | 复制 SSH 连接信息 |

### 连接与执行

| 命令 | 说明 |
|------|------|
| `sshm --ping, -p [别名\|ID]` | 测试主机连通性 |
| `sshm --exec, -x <目标> <命令>` | 在指定主机上执行命令 |
| `sshm --exec-group, --xg <分组> <命令>` | 在分组内所有主机执行命令 |
| `sshm --exec-all, --xa <命令>` | 在所有主机上执行命令 |

### 认证与安全

| 命令 | 说明 |
|------|------|
| `sshm --passwd <别名\|ID>` | 设置 SSH 密码（加密存储） |
| `sshm --forget-pass <别名\|ID>` | 删除已存储的密码 |
| `sshm --import-key <别名\|ID> <路径>` | 导入 SSH 私钥 |
| `sshm --gen-key <别名\|ID>` | 生成新的 SSH 密钥对 |
| `sshm --show-pubkey <别名\|ID>` | 显示公钥内容 |
| `sshm --auth <别名\|ID>` | 修改认证策略 |

### 配置管理

| 命令 | 说明 |
|------|------|
| `sshm --export-ssh-config [文件]` | 导出为 OpenSSH 配置格式 |
| `sshm --import-ssh-config [文件]` | 从 OpenSSH 配置导入主机 |

### 全局选项

| 选项 | 说明 |
|------|------|
| `--help, -h` | 显示帮助信息 |
| `--version, -v` | 显示版本信息 |

## 🛠 技术架构

```
sshm/
├── main.go                  # 入口
├── internal/
│   ├── command/             # CLI 命令处理
│   │   ├── root.go          # 命令路由 & 帮助
│   │   ├── interactive.go   # 交互模式
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

## 📄 许可证

[MIT License](LICENSE)

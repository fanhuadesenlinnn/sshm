# sshm v6.0.9

v6.0.9 修复了不同终端的 Backspace 兼容性，并重做了交互模式的远程命令边界解析。本版本不改变 v6 主配置或 Deploy schema，不需要迁移。

## 修复

- Backspace 同时兼容终端常见的 DEL (`0x7f`) 与 BS/Ctrl-H (`0x08`) 编码，直接按 Backspace 即可删除，命令输入和主机筛选行为一致。
- 修复交互模式对远程命令先拆分、去引号再重组，导致 `awk`、`printf`、JSON、Shell 变量与反斜杠语义变化的问题。

## 交互命令边界

- `x/exec` 和 `xt/exec-tag` 支持用 `--` 显式标记远程命令起点；后续文本作为一个不透明载荷原样传递。
- 本地选项可以写在目标前，也可以写在目标后、远程命令前，例如 `x web01 --quiet -- pwd`。
- 远程命令一旦开始，后续 `--quiet`、`--yes` 等文本都属于远程命令；需要明确边界时使用 `--`。
- 保留 `x host 'complete command'` 的旧用法兼容。普通管理命令现在会严格检查未闭合引号，并保留 Windows 路径反斜杠。

## 用法

```text
sshm> x web01 --quiet -- pwd
sshm> x web01 -- awk '{print $1}' /tmp/data
sshm> xt prod --parallel 4 --yes -- systemctl restart app
```

## 质量

- 新增 Backspace 双编码、Unicode 删除、原始命令保真、`--` 边界、目标前后选项、旧引号用法、未闭合引号与 Windows 路径回归测试。
- 通过全量测试、竞态检查、`go vet`、Staticcheck、Govulncheck 与构建验证。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.8 升级到 v6.0.9 不需要修改配置文件。

# sshm v6.0.3

v6.0.3 是一次使用体验与防误操作修复版本，不改变 v6 配置 schema，也不需要迁移。

## 改进

- `host`、`key`、`tag`、`logs` 和 `completion` 的 `--help` 现在直接展示可执行的子命令列表和示例，不再只显示一行 Usage。
- `sshm init` 和 `sshm add` 成功后会给出下一步命令，帮助新用户完成添加、验证和连接闭环。
- 空标签、空最近连接、搜索无结果、标签或主机未找到时，会给出更明确的后续命令或相近名称建议。
- `completion` 脚本和候选命令不再要求先初始化配置，安装补全时更顺手。
- 非交互环境运行无参数 `sshm` 时会显示帮助并退出，不会进入工作台读取空输入。

## 修复

- 拒绝 `--all` 与具体主机或 `--tag` 混用，避免本想操作单台主机时误扩大到全部主机。
- 单主机 `exec` 支持把 `--yes`、`--quiet` 和 `--no-log` 放在命令末尾；需要把同名参数传给远端命令时可使用 `--` 分隔。
- `logs unknown` 现在会报未知子命令，不再误当作列出日志。
- `push` / `pull` 正确尊重 `--` 分隔，避免路径名被误识别为 sshm 选项。
- `forget-pass`、`key delete`、`key delete-unused`、`logs clean` 和 `config host-key-policy insecure` 在非交互环境中必须显式使用 `--yes`。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.2 升级到 v6.0.3 不需要修改配置文件。
- 使用脚本清理日志、删除凭据或设置 `host-key-policy insecure` 时，请补充 `--yes`。

## 验收

```bash
go test ./...
go vet ./...
go build ./...
go test -race ./...
```

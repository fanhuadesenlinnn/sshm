# sshm v6.0.1

v6.0.1 是一次体验与诊断修复版本，不改变 v6 配置 schema，也不需要迁移。

## 改进

- 完善 `sshm --help` 和核心子命令帮助，增加首次使用路径、参数形状和可复制示例。
- `exec`、`exec-tag`、`push`、`pull`、`key`、`tag`、`deploy run` 等命令的 help 更接近真实操作流程。
- `key/tag/host` 中心命令的内层 `--help` 不再被当作普通参数错误。
- 未知命令或主机接近已有命令时，会给出更清晰的拼写建议。

## 修复

- 将“未配置凭据/托管密钥”拆分为独立 `credential` 失败阶段，避免误报成远端执行失败。
- 批量执行、ping 和 deploy 会把 credential 类失败归为 `unreachable`，退出码保持连接失败语义。
- 托管密钥需要解锁 vault 时保留 `vault` 阶段，不再被 credential 覆盖。
- 交互连接在缺少凭据时仍可提示输入一次临时 SSH 密码。
- `exec-tag` 和 `deploy run` 的进度只在真实终端中原地刷新，避免 CI 或管道日志出现回车进度碎片。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.0 升级到 v6.0.1 不需要修改配置文件。

## 验收

```bash
go test ./...
go vet ./...
go test -race ./...
```

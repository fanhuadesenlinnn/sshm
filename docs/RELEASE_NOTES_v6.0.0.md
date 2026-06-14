# sshm v6.0.0

v6.0.0 是一次破坏性重构。产品版本为 `v6.0.0`，Go module 为 `github.com/fanhuadesenlinnn/sshm/v6`，主配置与 Deploy schema 均升级为 `version: 2`。

## 主要变化

- 默认数据目录统一为 `~/.sshm`，只保留 `SSHM_HOME`。
- 新增 `sshm init`、`sshm config path`，旧配置只提示、不迁移。
- 引入 Cobra 命令树，保留工作台与 alias/ID 直连。
- 使用 `exec-tag/push-tag/pull-tag all` 替代旧 `*-all` 命令。
- 新增共享 BatchRunner，统一 serial、parallel、失败阈值、skipped、取消和退出码。
- 重构 push/pull：默认 SHA-256、目录 manifest、backup、临时目标、原子 rename、多主机 fetch 路径和冲突检查。
- rsync 仅在可保持 SFTP 同等语义时使用，否则 auto 回退 SFTP、显式 rsync 失败。
- Deploy 重写为严格 v2 嵌套 action DSL，支持 plan/check/diff、handlers、ignore_error、conditions 和 become。
- 操作日志支持关闭与 retention，Deploy diff 默认不写入日志。

## 升级注意

- v6 不读取、迁移或删除旧 `~/.config/sshm/sshm.yaml`。
- 主配置必须通过 `sshm init` 生成或手工转换为 schema v2。
- Deploy v1 文件必须手工改写为 v2。
- 当前目录 `sshm.deploy.yaml` 不再隐式加载，请使用 `--file`。

## 验收

```bash
go test ./...
go vet ./...
go build ./...
go test -race ./...
go list -m
```

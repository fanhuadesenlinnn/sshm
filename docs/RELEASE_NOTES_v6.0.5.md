# sshm v6.0.5

v6.0.5 是一次发布前加固版本，不改变 v6 主配置或 Deploy schema，也不需要迁移。

## 修复

- 加固主配置文件锁：持锁进程会刷新锁文件心跳，旧锁只有在超过 stale 时间且持锁 PID 不存活时才会被清理，避免长事务被误判为 stale 后并发写入。
- 明确 Deploy `confirm` 语义：`confirm` 现在在计划输出中标记为 `batch gate`，运行结果会说明它已在当前 serial 批次开始前确认。

## 质量

- 补充文件锁心跳和死亡旧锁清理的回归测试。
- 补充 rsync 能力探测与 fallback 契约测试：`auto` 在无法保证 v6 安全语义时回退 SFTP，显式 `--method rsync` 会失败。
- 保留 rsync 真实远端 `rsync --server` 高保真测试环境作为后续发布项。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.4 升级到 v6.0.5 不需要修改配置文件。
- Deploy `confirm` 的用户可见语义现在明确为 serial 批次门禁；如果需要在每台主机的某个命令后做人工判断，请继续使用普通交互流程或拆分 profile。

## 验收

```bash
go test ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...
go build ./...
go test -race ./...
go list -m
```

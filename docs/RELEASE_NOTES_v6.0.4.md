# sshm v6.0.4

v6.0.4 是一次质量与首次使用路径修复版本，不改变 v6 主配置或 Deploy schema，也不需要迁移。

## 改进

- 未初始化时直接运行 `sshm` 现在会展示首次使用引导和下一步命令，不再只返回初始化错误。
- `deploy init` 生成示例后会提示 `validate`、添加带 `prod` 标签主机和生成计划的后续命令。
- 没有主机时，`deploy validate` 和 `deploy list` 可以先检查示例配置结构；真正 `plan/run` 仍会要求可解析目标，并给出添加主机或修改 targets 的提示。
- GitHub Release 现在读取仓库内的版本发布说明，发布页会包含完整用户说明，而不是只有平台和安装信息。

## 修复

- 普通 `push` / `pull` 与 Deploy 统一远端路径安全规则，拒绝空路径、根路径、`~` 和上级目录组件。
- `logs clean` 增加日志目录归属检查，拒绝在 `SSHM_HOME` 指向文件系统根目录等危险配置下执行清理。
- 清理 staticcheck 发现的死代码、无效测试赋值和错误文本问题。
- 为显式用户控制的 `EDITOR`、rsync 和 insecure host trust 路径补充安全审计注释。

## 发布质量

- CI 新增 `staticcheck` 和 `govulncheck`。
- 发布流水线在打包前也会运行 `staticcheck` 和 `govulncheck`。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.3 升级到 v6.0.4 不需要修改配置文件。
- 如果脚本中依赖 `push` / `pull` 使用 `../` 形式的远端路径，请改成明确的远端路径。

## 验收

```bash
go test ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...
go build ./...
go test -race ./...
go run github.com/rhysd/actionlint/cmd/actionlint@latest
```

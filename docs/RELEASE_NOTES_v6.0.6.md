# sshm v6.0.6

v6.0.6 是一次安全性和可恢复性加固版本，不改变 v6 主配置或 Deploy schema，也不需要迁移。

## 修复

- 加固 `sshm key revoke` 的远端 `authorized_keys` 更新流程：只有在成功生成完整临时文件后才替换原文件，避免 grep 或写入失败时把远端 key 文件清空或截断。
- 修复 Deploy 失败后的重试命令：现在会保留原始 `--check`、`--diff`、多 `--file`、批处理限制、失败阈值和超时设置，避免 dry-run 失败后给出会真实执行的重试命令。
- 修复 `exec`、`push` 和 `pull` 失败后的重试命令 shell quoting，避免包含引号、空格或命令替换字符的主机名、命令和路径在复制粘贴后被本地 shell 错误解释。
- 修复传输重试命令遗漏显式 `--method` 的问题，确保用户选择 `sftp` 或 `rsync` 时重试路径保持一致。
- `sshm key create` 和 `sshm key import` 现在会拒绝未知尾随参数，避免拼写错误被静默忽略。
- 内部配置更新不再在配置文件缺失时隐式创建默认配置，保持“必须先 `sshm init`”这一边界只由初始化流程完成。

## 质量

- 补充 key revoke 远端脚本错误保留、retry 命令 shell-safe quoting、Deploy dry-run retry 语义和 key create/import 参数校验的回归测试。
- 补齐配置仓库严格初始化后的 command、secret、sshx 和 config 测试夹具，防止测试继续依赖隐式创建配置的旧行为。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.5 升级到 v6.0.6 不需要修改配置文件。
- 如果你复制失败输出中的 retry 命令，现在它会更忠实地复现原始操作上下文，尤其是 Deploy 的 `--check` dry-run。

## 验收

```bash
go test ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...
go build ./...
go test -race ./...
go test -cover ./...
```

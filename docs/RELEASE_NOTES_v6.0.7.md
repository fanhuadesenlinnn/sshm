# sshm v6.0.7

v6.0.7 是一次真实流程可用性和测试可执行性改进版本，不改变 v6 主配置或 Deploy schema，也不需要迁移。

## 新增

- 新增 `docs/AI_FLOW_TEST_PLAN.md`，把 sshm 的本地、交互、远程、传输、Deploy、日志和负例流程整理成可交给 AI/脚本逐条执行的测试清单。
- 新增 `SSHM_MASTER_PASSWORD` 环境变量，用于在非交互环境解锁已有 vault。它只在当前进程环境中使用，不会写入配置或输出日志，适合临时测试环境和 CI。

## 修复

- 修复非交互远程流程无法使用已保存密码的问题：`ping`、`exec`、`exec-tag`、`push`、`pull`、`deploy run` 等现在可通过 `SSHM_MASTER_PASSWORD` 解锁 vault。
- 修复需要 vault 凭据但无法交互解锁时的错误提示，现在会明确提示设置 `SSHM_MASTER_PASSWORD`。
- 修复传输失败后的 retry 建议：当目标已存在且内容不同，重试命令会默认追加更保守的 `--backup`。
- 修复传输失败建议文案，现在同时提示检查 `--overwrite/--backup`，避免只引导用户覆盖。
- 修复 `deploy init -f <file>` 生成后的下一步提示，后续 `validate` 和 `plan` 命令会保留同一个 `-f <file>`。

## 体验改进

- `sshm add` / `sshm host add` 成功后会回显标签和备注，方便用户确认 `--tags` 与 `--note` 已生效。
- 非交互删除主机但未传 `--yes` 时，不再先打印完整主机详情，直接给出确认参数提示。

## 实机验收

- 使用临时 `SSHM_HOME` 和一台真实 SSH 服务器验证了 `ping`、`exec`、`exec-tag`、SFTP 文件推送/拉取、目录推送/拉取、覆盖保护、`--overwrite`、远端临时目录清理和批量 ping。
- 实机测试仅使用远端 `/tmp/sshm-ai-test-*` 临时目录，测试后已清理。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.6 升级到 v6.0.7 不需要修改配置文件。
- `SSHM_MASTER_PASSWORD` 适合非交互自动化；不要把真实个人主密码写入共享 shell 历史、CI 日志或长期配置文件。

## 验收

```bash
go test ./...
go vet ./...
```

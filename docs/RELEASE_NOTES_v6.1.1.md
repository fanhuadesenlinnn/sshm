# sshm v6.1.1

v6.1.1 修复一批使用层面的问题，主配置与 Deploy schema 均无变化。

## 修复

- `sleep`/`wait_for` 不再被任务级默认 30 秒超时截断：两个模块自带时长边界，执行时跳过通用任务超时包裹，`sleep: 40s` 与 `wait_for: timeout: 60s` 按声明时长生效（Ctrl+C 取消仍然有效）。
- `export-ssh-config` 导出跳板机（`ProxyJump`）；普通身份文件路径同时导出（仅托管密钥仍以注释提示）。
- 单主机 `sshm ping` 不可达时退出码对齐为 2（与多主机 ping 及文档约定一致），并写入操作日志。
- `sshm search` 在非交互环境缺少关键词时直接报用法，不再阻塞等待 stdin。

## 质量

- 新增回归测试：sleep 不受短任务超时影响、导出包含 ProxyJump、单主机 ping 退出码 2、search 非交互用法错误。
- 通过全量测试、竞态检查、`go vet`、Staticcheck、Govulncheck 与构建验证。

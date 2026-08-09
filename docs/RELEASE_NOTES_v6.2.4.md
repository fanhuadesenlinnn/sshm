# sshmd v6.2.4

v6.2.4 是一次由真实 Ubuntu 虚拟机全功能回归驱动的可靠性与结果语义修复。主配置仍为 schema v2，Deploy 仍为 schema v3；本版本不增加新的运维权限边界，重点消除会造成错误结果、重复变更、认证中断和本地目录误覆盖的实际问题。

## 传输与 OpenSSH 配置

- 修复 rsync 目录 push/pull 缺少源目录尾分隔符导致 checksum 校验失败；目录内容现在与 SFTP 使用相同布局。
- 单主机目录 pull 的明确目标在首次与后续运行中保持一致，不再在第二次运行时多嵌套一层远端目录名。
- 当前目录、用户主目录、`SSHMD_HOME` 及其祖先保留容器语义，并在激活前再次拒绝整体替换，避免 `pull ... . --overwrite` 移动或删除工作目录。
- OpenSSH 导入保留可映射的单级 `ProxyJump`，支持常见的 `Host *`/通配符默认项与 OpenSSH 的首值优先规则；`Match` 条件不会再误污染前一个主机。
- OpenSSH 导出默认拒绝覆盖已有文件；明确确认后可使用 `--force`。

## 密钥、日志与诊断

- `key revoke` 在远端撤销成功后解除对应主机的失效本地绑定；部分失败主机保留原绑定，并给出替代凭据的明确下一步。
- `logs --host ... --action deploy` 会读取 Deploy `plan.json`/`run.json` 中的目标主机，不再漏掉 Deploy 日志；无匹配时给出明确空状态。
- 成功的 Deploy 主机结果不再携带通用故障建议；本地模块配置错误保留 `stage=config` 并统一退出 3。
- 用户拒绝 pause/confirm 使用独立的 `stage=confirmation`、准确建议和退出 1，不再误报为 playbook 配置错误。

## Deploy 收敛与结果语义

- `copy` 支持显式空字符串内容，可可靠创建和收敛空文件。
- `file` 仅在 owner/group 实际不一致时执行 chown，修复每次运行都 changed；属主和目录 mode 探针同时兼容 GNU/Linux 与 BSD/macOS `stat`。
- `unarchive` 的一致性比较覆盖路径、类型、mode 和文件哈希，可识别额外链接、特殊条目、空目录和权限漂移；目标有漂移时执行完整目录切换。
- block 的 `always` 子任务会正确汇总 changed/would-change；同文案的不同 pause/confirm 任务分别门禁，而同一任务跨主机仍只确认一次。
- free strategy 会缓存并传播确认拒绝，所有等待主机都停止，任何主机都不会绕过拒绝继续执行。
- loop register 保存每个实际迭代的结构化结果，不再返回 null 列表。
- `command`/`shell` 实际执行成功默认报告 changed，`creates`/`removes` 跳过保持 OK，`changed_when` 可显式覆盖；`check_safe` 在 check 模式默认保持只读 OK。
- `ignore_errors` 在 JSON 中保留 `ignored: true`、原始 rc 和阶段，便于自动化调用者区分真实成功与已忽略失败。

## 发布验证

- 在 OrbStack Ubuntu 24.04 ARM64 虚拟机上覆盖 strict/accept-new host trust、托管密钥 create/import/use/setup/status/revoke、单机与批量 exec/ping、SFTP/rsync push/pull、端口转发、密码 sudo 正反路径、日志与 recent。
- Deploy 13 个模块均完成成功、关键失败、check/diff 与第二次幂等回归；所有本版本确认问题均以补丁后二次黑盒测试验证。
- 本地门禁覆盖全量单元/集成测试、race、vet、staticcheck、govulncheck、安装器检查和六个平台交叉构建。

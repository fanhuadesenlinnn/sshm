# sshm v6.1.2

v6.1.2 集中修复一批实机测试发现的缺陷，重构连接管理为唯一入口，并新增两类凭据能力：become 密码提权与主配置明文密码（与加密 vault 并存）。主配置 schema 仍为 `version: 2`，Deploy schema 为 `version: 3`，无需迁移。

## 修复

- 任务级 `confirm` 字段无法解析（v6.1.0 新特性不可用）：解析器已知键遗漏导致任何带 confirm 的 playbook 校验失败；独立 confirm 任务现在给出“必须与模块搭配”的明确提示。
- `--exclude/--exclude-tag` 覆盖语义错误：exclude-only 覆盖曾整体替换 play 的 hosts，误报“hosts 不能为空”；现在与 play 目标合并，排除后为空报“排除后目标主机为空”。
- `exec-tag` 未知标签报错误导：排除逻辑先于空目标检查，把“没有匹配标签”误报为“排除后目标主机为空”。
- `logs --action` 过滤匹配不到批量/传输/deploy 日志：目录名如 `-exec-batch`、`-push-batch`、`-deploy-publish` 未被识别，现支持“后缀或中缀”双匹配。
- sleep 在 check 模式的跳过原因不再误导性建议 `check_safe`（sleep 不支持该字段）。

## 连接管理重构

- 新增统一会话管理器（`sessionManager`）：Exec、传输、Stat、DialTCP 全部走同一获取路径，不再存在旁路拨号。
- `Stat` 复用缓存会话与其共享 SFTP 通道，不再每次新建连接；一次 run 内每台主机一条连接。
- 连接失效自动恢复：连接级错误（network/session）会丢弃失效会话，下一个任务自动重拨；失败任务本身不重试，避免重复执行副作用。

## become 密码提权

- `become` 不再要求免密 sudo：sudo 需要密码时，密码可来自任务级 `become_password`、环境变量 `SSHM_BECOME_PASSWORD`，或自动复用该主机 vault 中的 SSH 密码。
- 密码始终经 stdin 传入 `sudo -S`，不出现在命令行或日志中。

## 主配置明文密码

- 主机支持明文 `password` 字段，与 `password_ref`（vault）互斥并存；明文主机无需 vault 即可连接。
- `sshm add --password`、`sshm passwd` 一键将明文加密进 vault 并清掉明文字段、`forget-pass` 同时清理两种来源。
- `doctor` 对明文密码主机给出提醒；Deploy 编排文件仍禁止保存凭据。
- 新增 ADR 0016 记录该决策及安全边界。

## 质量

- 新增回归测试：confirm 解析、exclude 合并、include 嵌套与循环检测、连接复用与断连重拨、become 密码三种来源、明文密码解析与升级、logs 过滤。
- 全量测试、`go vet`、Staticcheck、Govulncheck 与构建验证通过。

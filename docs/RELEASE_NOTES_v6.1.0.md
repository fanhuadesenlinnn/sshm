# sshm v6.1.0

v6.1.0 移除 Deploy v2，只保留 Deploy v3 单一执行引擎。主配置 schema 仍为 `version: 2`，Go module 仍为 `/v6`。

## 破坏性变更：Deploy v2 移除

- `version: 2` 的 profile/steps/handlers 不再加载，加载旧文件会直接报错。
- `deploy migrate` 命令与全部 v2 导入路径已删除，不存在兼容转换。
- v2 的 `wait`（定时延时）由 v3 新增的 `sleep` 模块替代。
- v2 的 `confirm`（serial 批次门禁）由 v3 任务级 `confirm` 字段替代，语义一致：linear 策略下每个批次开始前确认，check 模式跳过。

## 简化

- Deploy v2 的 `internal/deploy` 包删除；v3 引擎与共享的目标选择/条件类型合并为单一的 `internal/deploy` 包。
- `deploy` 命令不再做文件版本检测，全部命令只处理 v3 playbook；`deploy init --version 2` 移除。
- 文档与模板统一为 v3：README、CONTEXT、PRODUCT_DESIGN、ROADMAP 清理 v2 内容，新增 ADR 0015 记录移除决策。
- 历史命名收尾：`V3Event` 改名 `PlayEvent`，CLI 内的 `loadDeployV3`/`deployV3Overrides` 去掉 v3 后缀。

## 架构与功能改进

- 连接复用：一次 deploy run 内每台主机只建立一条 SSH 连接，Exec 与 wait_for 目标机检测共享会话；`NativeExecutor.CloseSessions` 在工作台等长驻进程内释放连接。
- `wait_for` 新增 `connect_from: controller|target` 与 `host` 字段：target 模式从目标机侧检测端口，跳板机/内网主机不再误判。
- `debug: var:` 现在能读取 register、facts 与 loop 变量，不再局限于 vars。
- `service` 模块识别 masked 服务：enabled/started/restarted 时先 unmask 再操作。
- `command` 模块与 `shell` 区分：`command` 不经过远程 shell，拒绝管道/重定向/`$`/`;`/`&`/反引号；需要 shell 语法用 `shell`。
- facts 缓存键改用稳定主机 ID，避免不同 alias 碰撞。
- vault 的 scrypt 参数提升到 `N=2^17`，旧参数 vault 在首次解锁时自动用新参数重加密（保持原 salt 与主密码）。

## 体验与质量

- 交互工作台首屏列出 x/xt、push/pull、forward、sc、passwd、logs 等核心命令。
- confirm 拒绝时的错误信息统一为 pause/confirm。
- 版本号不再硬编码：安装脚本帮助文本使用 `vX.Y.Z` 示例，README 安装示例改用 `latest`，CI 安装器测试动态取最近 tag。
- deploy run 失败时列出前 3 台失败主机/任务并提示 `--check` 复跑。
- AI 流程测试计划的 deploy 章节补上"写入真实 playbook"的准备步骤，修正与 init 模板（空 plays）脱节的问题。
- CI 新增 macOS/Windows 的 `go test`/`go vet`/`-race` job。
- 传输连接复用：push/pull 复用缓存的 SSH+SFTP 会话，文件密集型 playbook 不再每次传输新建连接。
- vault 升级策略调整：不再在读取时重加密（`doctor` 等只读命令零写副作用），下次写入时自动以当前 scrypt 参数重加密。
- 批量与 deploy 目标覆盖新增 `--exclude`/`--exclude-tag`，排除后目标为空或拼写错误会明确报错。
- 一次性确认（ReadYesNo 等）不再污染命令行历史。
- loop 任务支持按 item 求值 `when`（`when: item != 'x'` 过滤单项），被过滤项不进 register。
- `sshm exec` 单主机输出改为实时流式（`tail -f` 等长命令不再全程黑屏）；`--quiet` 仍只落日志不打印。
- `gather_facts` 改为受 `parallel` 限制的并发收集，大批量主机不再在任务开始前串行等待。
- 交互工作台 `x`/`xt` 自动确认（用户已显式输入命令），并支持 `--exclude`/`--exclude-tag`。
- `sleep` 模块加上限（最多 24 小时），超限在 plan 校验阶段报错。
- `--check` 模式不再写入 facts 缓存，保持只读。
- `deploy plan --output json` 增加每个任务的 `args`。
- ndjson 事件流在 linear/free 策略下统一为按任务×主机发 `task_host_done`（含 Task/TaskIndex）。
- `passwd`、`forget-pass`、`key` 批量命令支持 `--exclude`/`--exclude-tag`。
- `doctor` 增加 Deploy 配置加载与校验检查。
- `deploy run --check` 不再因 check 跳过的任务返回失败退出码，check 跳过原因单独标注（"check 模式跳过，可设 check_safe: true 执行"），失败提示区分是否已在 check 模式。
- `exec-tag` 每台主机完成即打印其输出，全部结束后仍显示完整汇总；`--quiet` 行为不变。
- `logs` 支持 `--host <别名>` 与 `--action <动作>` 筛选。
- `forward` 支持同一条连接内的多条本地端口转发（`<本地> <远程> [<本地> <远程> ...]`）。
- 文本 diff 改为真正的行级 unified diff（LCS + 上下文 hunk），超大文件自动回退全量替换展示。
- README 与初始化模板补充 `check_safe: true` 说明。
- 新增测试：会话复用与 ReusableSession 防护、wait_for 目标机检测、service unmask、debug register、command 元字符拒绝、scrypt 重加密、ssh_config 解析导出、搜索匹配、行编辑器状态机与表格渲染；`internal/ui` 覆盖率提升。

## 质量

- 新增 `sleep` 模块与任务级 `confirm` 的引擎测试（批次门禁、拒绝中止、check 跳过、free 策略去重确认）。
- 目标解析与条件匹配测试补齐；transfer 集成测试改写为基于 v3 引擎。
- 通过全量测试、竞态检查、`go vet`、Staticcheck、Govulncheck 与构建验证。

## 升级注意

- 持有 v2 deploy.yaml 的用户需要手工改写为 v3 playbook；`notify`/handlers 改写为 `register + when`。
- 主配置无 schema 变化，不需要迁移。

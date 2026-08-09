# sshmd v6.2.3

v6.2.3 是一次面向正式使用的安全、可靠性与首次体验加固。主配置仍为 schema v2，Deploy 仍为 schema v3；本版本收紧了不安全输入，修复了并发连接、超时、取消、配置事务与跨平台权限边界。

## 安全与数据完整性

- `command` 改为字面 argv 解析；带模板的动态参数必须使用结构化 `argv` 列表并逐元素安全引用，需要 shell 语法时必须显式使用 `shell`。
- Deploy 本地源、include、vars_files 与 fetch 目标被限制在顶层 playbook 项目内，拒绝绝对路径、目录逃逸和已有符号链接。
- `unarchive` 使用格式感知的 tar/zip 元数据校验，拒绝链接、特殊文件、setuid/setgid、加密 zip、路径穿越和超额解压；目录以 staged switch 激活，支持真实 backup，且不再残留旧版本文件。
- 移除会把秘密暴露到进程参数的 `add --password`，新增终端隐藏输入和管道友好的 `--password-stdin`。
- Deploy 文件中的 `become_password` 现在严格拒绝；sudo 密码只从 `SSHMD_BECOME_PASSWORD` 或主配置 vault 获取。
- 配置与锁事务改用操作系统文件锁；`config edit` 增加 compare-and-swap，避免并发修改被长时间编辑器静默覆盖。
- Windows 对配置、锁、临时密钥和状态目录设置当前用户与 LocalSystem 专属的受保护 DACL；同时拒绝把 `SSHMD_HOME` 指向文件系统根目录。

## 连接与运行可靠性

- 不同主机的首次 SSH 拨号真正并行；同一主机的并发拨号自动合并，不再被全局锁串行化。
- connect timeout 覆盖 TCP、跳板机、目标握手和 SSH 握手全链路；单主机 exec 开始遵循配置的命令超时。
- SFTP、远端 ping、TCP 转发和 rsync 能力探测均传播取消；取消后关闭并淘汰失效缓存会话。
- SSH、rsync 和聚合 diff 输出增加确定的内存上限及截断标记；本地 diff 不再整文件无界读取。
- 全量 ping 使用受控并发并返回准确退出码；doctor 对未初始化、错误主密码、缺失凭据和无效 Deploy 配置返回非零状态。
- free strategy 的人工确认串行化，修复 macOS/Windows race 测试暴露的并发终端提示问题。

## 首次使用与文档

- `deploy init --dir` 生成可立即 validate/plan 的安全只读 demo，并按 `add -> validate -> plan -> check` 给出可执行下一步。
- 新增主机后的指引调整为 `passwd -> ping -> connect`；OpenSSH `IdentityFile` 导入保留主机元数据，并输出可直接执行的托管密钥 import/use 命令。
- 单目标与零参数命令统一拒绝多余参数，避免拼写错误被静默忽略。
- README、生成模板、产品设计与上下文文档同步新的 command、become、路径和 archive 安全边界。

## 发布质量

- 补齐并发拨号、取消、超时、输出上限、恶意归档、路径逃逸、配置冲突、Windows 权限和 CLI 流程回归测试。
- 修复 CI 浅克隆取不到历史 tag 导致的安装器误判；Release 工作流只允许 `v*` tag 触发，避免从 main 手动生成错误版本。

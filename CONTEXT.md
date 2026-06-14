# sshm v6 Context

sshm 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。核心用户独立管理约 5 到 100 台主机，不需要团队权限、集中审计或后台控制平面。

## 版本边界

- 产品发布版本：`v6.0.0`
- Go module：`github.com/fanhuadesenlinnn/sshm/v6`
- 主配置 schema：`version: 2`
- Deploy schema：`version: 2`

这些版本含义不同。主配置和 Deploy schema v2 均不自动兼容或迁移旧版本。

## 核心语言

**数据目录**
sshm 拥有的全部本地状态根目录。默认是 `~/.sshm`，只允许通过 `SSHM_HOME` 整体覆盖。

**主配置**
`<SSHM_HOME>/sshm.yaml`。它是唯一权威可变状态，保存 defaults、hosts、tags、managed_keys、host_trust 和加密 vault。

**主机**
核心用户通过 SSH 连接和操作的一台远程服务器。主机拥有稳定 ID 和跨平台安全 alias。

**标签**
主机的可多选分类。`all` 是批量命令使用的虚拟保留标签，不可创建。

**目标集合**
一次远程操作解析出的稳定有序主机列表，不持久化。

**主机信任**
核心用户确认并由 sshm 持续验证的远端主机身份。`--yes` 不会跳过主机信任。

**托管密钥**
导入并保存在加密 vault 中的 SSH 私钥。普通私钥路径只作为导入源。

**主密码会话**
当前 sshm 进程成功解锁 vault 后，到 `lock` 或进程退出为止的凭据使用时间段。

**执行确认**
用户看到动作和目标集合后，对一次远程操作的批准。`--yes` 仅跳过这一层确认。

**BatchRunner**
`exec-tag`、`push-tag`、`pull-tag` 与 `deploy run` 共享的批量调度器。它负责稳定顺序、serial、parallel、失败阈值、取消、skipped 和结果聚合，不负责凭据解锁、host trust 或操作确认。

**逐主机结果**
每个目标主机的 `ok`、`changed`、`would-change`、`failed`、`unreachable` 或 `skipped` 结果。

**安全传输**
push/pull 使用 manifest、SHA-256、临时目标和 rename，拒绝符号链接与特殊文件。rsync 只能作为保持相同语义的可选加速路径。

**Deploy 文件**
用户维护的只读 YAML 输入。默认位置是 `<SSHM_HOME>/deploy.yaml` 和 `<SSHM_HOME>/deploy.d/*.yaml`；项目文件必须通过 `--file` 显式指定。

**Deploy Profile**
一个命名的 Deploy v2 工作流，包含 targets、批量策略、steps 和可通知 handlers。

**执行计划**
Deploy Profile 静态解析出的来源文件、目标主机、steps、handlers 和批量参数。计划生成不连接远端。

## 核心关系

- 一个核心用户拥有一个数据目录。
- 一个主配置包含多个主机、标签、托管密钥、主机信任条目和一个可为空的 vault。
- 一台主机可拥有多个标签并引用一个托管密钥、密码引用和单级跳板机。
- 一个目标集合包含一个或多个稳定有序主机。
- 一个批量操作为目标集合中的每台主机产生一个逐主机结果。
- 一个 Deploy Profile 解析为一个执行计划，并通过共享 BatchRunner 运行。
- 一个普通 step 结果为 `changed` 时可以通知一个或多个 handler；同一 handler 每台主机只执行一次。

## 不变量

1. sshm 不读取、迁移或删除旧 `~/.config/sshm/sshm.yaml`。
2. 除 `init`、`config path` 和 `doctor` 外，缺少主配置时不得静默创建。
3. 主配置与 Deploy v2 严格拒绝缺失版本和未知字段。
4. `--yes` 不跳过主密码和 host trust。
5. 批量操作在调度任何主机前完成所需 vault 解锁。
6. 多主机 pull 在下载前完成路径安全与冲突检查。
7. 文件传输失败时不得把半成品暴露为最终目标。
8. Deploy `plan` 不连接远端；`check` 不修改最终目标。
9. diff 可能包含敏感内容，默认不写入操作日志。
10. sshm 不提供后台任务、团队空间、完整 Ansible 兼容层或期望状态收敛。

## 接口

- 无参数 `sshm`：轻量工作台。
- `sshm <alias|ID>`：最快直连路径。
- Cobra CLI：可重复执行的完整命令接口。

三种接口共享同一主配置、目标模型、安全策略和操作语义。

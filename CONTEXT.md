# sshmd v6 Context

sshmd 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。核心用户独立管理约 5 到 100 台主机，不需要团队权限、集中审计或后台控制平面。

## 版本边界

- 产品发布版本：`v6.2.3`
- Go module：`github.com/fanhuadesenlinnn/sshmd/v6`
- 主配置 schema：`version: 2`
- Deploy schema：`version: 3`

主配置与 Deploy schema 相互独立；Deploy 仅支持 v3 playbook。

## 核心语言

**数据目录**
sshmd 拥有的全部本地状态根目录。默认是 `~/.sshmd`，只允许通过 `SSHMD_HOME` 整体覆盖。

**主配置**
`<SSHMD_HOME>/sshmd.yaml`。它是唯一权威可变状态，保存 defaults、hosts、tags、managed_keys、host_trust、加密 vault，以及可选的明文密码字段。

**主机**
核心用户通过 SSH 连接和操作的一台远程服务器。主机拥有稳定 ID 和跨平台安全 alias。

**标签**
主机的可多选分类。`all` 是批量命令使用的虚拟保留标签，不可创建。

**目标集合**
一次远程操作解析出的稳定有序主机列表，不持久化。

**主机信任**
核心用户确认并由 sshmd 持续验证的远端主机身份。`--yes` 不会跳过主机信任。

**托管密钥**
导入并保存在加密 vault 中的 SSH 私钥。普通私钥路径只作为导入源。

**主密码会话**
当前 sshmd 进程成功解锁 vault 后，到 `lock` 或进程退出为止的凭据使用时间段。

**执行确认**
用户看到动作和目标集合后，对一次操作的批准。`--yes` 仅跳过这一层确认，不跳过主密码或 host trust。

**BatchRunner**
`exec-tag`、`push-tag`、`pull-tag` 与 `deploy run` 共享的批量调度器。它负责稳定顺序、serial、parallel、失败阈值、取消、skipped 和结果聚合，不负责凭据解锁、host trust 或操作确认。

**逐主机结果**
每个目标主机的 `ok`、`changed`、`would-change`、`failed`、`unreachable` 或 `skipped` 结果。

**安全传输**
push/pull 使用 manifest、SHA-256、临时目标和 rename，拒绝符号链接与特殊文件。rsync 只能作为保持相同语义的可选加速路径。

**Deploy 文件**
用户维护的只读 YAML 输入。默认位置是 `<SSHMD_HOME>/deploy.yaml` 和 `<SSHMD_HOME>/deploy.d/*.yaml`；项目文件必须通过 `--file` 显式指定。

**v3 Playbook**
Deploy v3 声明文档，包含全局 vars 和多个 plays；文件结构沿用 `deploy.yaml` + `deploy.d/*.yaml`。

**v3 Play**
命名的 v3 工作流，包含 hosts、strategy（linear/free）、批量策略、vars（含 vars_files）与 tasks。

**v3 Task**
一个模块调用或 block/rescue/always 结构，可携带 when、register、loop、become、ignore_errors。

**v3 模块**
带幂等语义的执行单元：command/shell、file、copy、template、service、wait_for、sleep、unarchive、fetch、pause、fail、debug。每个模块内置 check/diff 与 changed 判定。

**register/when**
task 的结果（changed/rc/output）注册到主机状态，供后续 task 的 when 条件判断。

**静态 include**
任务片段与 vars_files 的引用机制；在 validate/plan 阶段展开，带循环检测，相对路径按片段文件自身目录解析。

**执行计划**
一个 v3 Play 静态解析出的来源文件、目标主机、tasks 和批量参数。计划生成不连接远端。

## 核心关系

- 一个核心用户拥有一个数据目录。
- 一个主配置包含多个主机、标签、托管密钥、主机信任条目和一个可为空的 vault。
- 一台主机可拥有多个标签并引用一个托管密钥、密码引用和单级跳板机。
- 一台主机要么引用加密 vault 凭据（`password_ref`），要么显式写明文 `password` 字段，两者互斥。
- 一个目标集合包含一个或多个稳定有序主机。
- 一个批量操作为目标集合中的每台主机产生一个逐主机结果。
- 一个 v3 Play 解析为一个执行计划，并通过共享 BatchRunner 运行。
- 一个 v3 Playbook 包含多个 plays；一个 play 解析为一个 v3 执行计划。
- 一个 v3 task 通过模块产生结果；register 的结果供后续 task 的 when 使用。
- v3 任务按任务遍历主机（linear）或按主机遍历任务（free），两者都复用共享 BatchRunner。

## 不变量

1. 数据目录仅使用 `~/.sshmd`，唯一路径覆盖变量是 `SSHMD_HOME`。
2. 除 `init`、`config path` 和 `doctor` 外，缺少主配置时不得静默创建。
3. 主配置与 Deploy v3 严格拒绝缺失版本和未知字段。
4. `--yes` 不跳过主密码和 host trust。
5. `--all` 不能与具体主机或标签选择器混用。
6. 批量操作在调度任何主机前完成所需 vault 解锁。
7. 多主机 pull 在下载前完成路径安全与冲突检查。
8. 文件传输失败时不得把半成品暴露为最终目标。
9. Deploy `plan` 不连接远端；`check` 不修改最终目标。
10. diff 可能包含敏感内容，默认不写入操作日志。
11. sshmd 不提供后台任务、团队空间、完整 Ansible 兼容层或期望状态收敛。
12. Deploy 文件必须使用 `version: 3`。
13. Deploy v3 的 `when` 引用未定义变量是错误而非静默跳过；可选变量必须显式使用 `is defined`。
14. v3 不提供 handlers；条件执行必须通过 register + when 表达。
15. v3 include 在 plan 阶段静态展开，不提供运行时动态 include。
16. 明文密码是主配置显式支持的可选方式（受 0600 权限保护）；Deploy 编排文件始终禁止保存密码或私钥。

## 接口

- 无参数 `sshmd`：未初始化时显示首次使用引导；初始化后进入轻量工作台。
- `sshmd <alias|ID>`：最快直连路径。
- Cobra CLI：可重复执行的完整命令接口。

三种接口共享同一主配置、目标模型、安全策略和操作语义。

# sshm v6.0.11

v6.0.11 引入 Deploy v3 模块化编排：从"按主机执行步骤"升级为"声明式模块执行器"，并补齐了实机测试发现的一批真实缺陷。主配置 schema 仍为 `version: 2`；Deploy 同时支持 `version: 2` 与 `version: 3`，v2 完全保留，无需迁移。

## Deploy v3 编排（新）

- 新 schema：`version: 3`，playbook 由 plays（工作流）、tasks（任务）、modules（模块）组成；文件结构沿用 `deploy.yaml` + `deploy.d/*.yaml`。
- 12 个幂等模块：command/shell、file、copy、template、service、wait_for、unarchive、fetch、pause、fail、debug；每个模块内置 check/diff 与 changed 判定。
- `register + when` 取代 handlers 表达条件执行；支持 `loop`、`run_once`、`ignore_errors`、`failed_when/changed_when`、become。
- `block/rescue/always` 提供同主机失败回滚结构。
- `strategy: linear/free` 控制跨主机执行顺序，serial/parallel/fail-fast/max-fail 复用共享 BatchRunner。
- 变量体系：文件级 vars、play 级 vars、`vars_files`、CLI `--extra-var`；`{{ }}` 插值与白名单函数（default/join/upper/lower/trim/replace）。
- 最小 `gather_facts`：hostname/system/arch/os_family，带缓存。
- 静态 `include`：任务片段与变量文件复用，带循环检测。
- 输出支持 text、json 与 ndjson 事件流；`deploy migrate` 把 v2 profile 机械转换为 v3 play。

## 实机测试修复

- 修复 Windows 客户端 pull 的 checksum 误判：manifest 比较不再包含文件 mode，内容一致即通过校验。
- 修复 fail-fast 触发后未启动主机的状态记录（此前显示 ok 且无任务记录，现为 skipped 并带原因）。
- 修复 copy/template 相对路径未按配置文件目录解析、失败主机后续任务缺失、运行时参数未合并 facts。
- 修复 ndjson/json 模式下模块原始输出污染机器可读流。
- 失败原因现在附带远端 stderr 首行，排查更直观。
- gather_facts 在缺少 `hostname` 命令的机器上回退读取 `/etc/hostname`；模板读取剥离 UTF-8 BOM。
- 支持 `{{ port | default 8080 }}` 这类 Ansible 惯用写法（此前会硬报错）。

## 初始化体验

- `sshm init` 默认生成 Deploy v3 注释模板（快速开始、字段逐项说明、全部模块示例、register/when、block/rescue、include、安全边界），初始化后即可通过 `deploy validate`。
- 新增 `templates/` 目录与可运行的模板示例，与编排示例互相呼应。
- 新增 `README.md` 一页速查：文件说明、常用命令、安全边界。
- `deploy init` 支持 `--version 2|3`，v2 模板原样保留。

## 升级注意

- 主配置仍为 `version: 2`；Deploy v2 文件继续受支持，与 v3 按文件版本并存，同一批文件版本混合时拒绝执行。
- v3 不提供 handlers（notify 在迁移时给出警告），条件执行请使用 `register + when`。
- v2 用户可继续使用；需要新能力时用 `sshm deploy migrate` 转换。

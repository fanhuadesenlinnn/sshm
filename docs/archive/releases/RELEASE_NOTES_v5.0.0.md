# sshm v5.0.0

## Deploy 工作流

- 新增可组合的多文件 deploy 配置和命名 Profile。
- 支持 `copy` 与 `exec` 顺序编排、按主机并发执行和逐步骤结果。
- 新增 `deploy init/validate/list/show/plan/run/exec/copy`。
- 支持 hidden 汇总、visible 实时输出、JSON 输出、超时、取消与受限复制重试。
- deploy 文件只描述任务，主机、凭据、密钥和信任继续由 `sshm.yaml` 管理。

## 执行与可靠性

- 提取共享 `internal/ops` 执行接口，现有 exec、push、pull 与 deploy 复用相同能力。
- 补充 timeout、config、vault 和 unknown 失败阶段。
- JSON 模式将提示、进度与实时输出写入 stderr，保持 stdout 可解析。

## 升级

- Go 模块主版本路径更新为 `github.com/fanhuadesenlinnn/sshm/v5`。
- `sshm.yaml` 结构保持不变，不需要迁移。
- deploy 文件属于可信输入，变量会直接替换到远程命令中，不进行 Shell 转义。

# sshm v6.0.10

v6.0.10 改进首次初始化和配置自解释能力。新用户执行一次 `sshm init` 即可获得带中文说明的主配置与安全 Deploy 模板；本版本不改变主配置或 Deploy schema，不需要迁移。

## 初始化体验

- `sshm init` 现在同时生成 `sshm.yaml`、`deploy.yaml` 和 `deploy.d/` 等工作目录。
- 自动生成的 `deploy.yaml` 使用合法的空 `profiles`/`handlers`，完整工作流示例全部注释，不会出现可被误执行的占位部署任务。
- 已有 `deploy.yaml` 属于用户维护的声明输入，即使执行 `sshm init --force` 也不会被覆盖；覆盖仍必须显式使用 `sshm deploy init --overwrite`。
- 初始化后的空 Deploy 模板可以直接通过 `sshm deploy validate` 和 `sshm deploy list`。

## 配置说明

- 主配置增加快速开始、手工主机示例、密码与托管区域安全边界说明。
- `defaults`、`tags`、`hosts`、`managed_keys`、`host_trust` 和 `vault` 增加贴近字段的中文注释。
- 官方注释通过 YAML 节点稳定生成，添加主机、修改凭据或其他保存操作后仍然存在。
- 明确说明自定义文字应写入主机或标签的 `note` 字段，不依赖保存时无法保留的自由 YAML 注释。
- `sshm deploy init` 生成的可运行示例补充目标选择、批量参数、action、handler、凭据边界和执行前检查说明。

## 质量

- 增加初始化 Deploy 文件、`0600` 权限、严格加载、安全空工作流和 `--force` 不覆盖用户 Deploy 配置的回归测试。
- 增加主配置快速开始、字段注释和 Deploy 示例说明的回归检查。
- 通过全量测试、竞态检查、`go vet`、Staticcheck、Govulncheck 与构建验证。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 已有安装不需要修改任何配置；如果尚未创建 Deploy 文件，可继续使用 `sshm deploy init` 按需生成。

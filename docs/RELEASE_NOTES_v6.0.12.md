# sshm v6.0.12

v6.0.12 调整交互命令的快捷映射：`p` 现在是 ping，主机选择器更名为 `find-con`（别名 `f`，保留 `pick` 兼容）。主配置与 Deploy schema 不变，无需迁移。

## 命令映射调整

- `p` / `ping`：测试连接。此前 `p` 是选择器别名，容易与 ping 混淆。
- `find-con`：查找并连接主机（新主名），别名 `f` 和 `pick` 均可用。
- 交互工作台、CLI、补全、帮助文本全部同步。

## 质量

- 交互命令分发抽为可测试函数，新增路由测试（p/ping、f/find-con/pick、q 退出）与补全候选测试。
- 通过全量测试、`go vet`、Staticcheck、Govulncheck 与构建验证。

## 升级注意

- 主配置与 Deploy 配置均无 schema 变化。
- 老用户可继续使用 `pick`；新脚本建议改用 `find-con` 或 `f`。

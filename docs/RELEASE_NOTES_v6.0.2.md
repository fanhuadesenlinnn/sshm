# sshm v6.0.2

v6.0.2 是一次 Windows 便携性与命令体验修复版本，不改变 v6 配置 schema，也不需要迁移。

## 改进

- Windows 风格的 `~\path` 现在会和 `~/path` 一样展开到用户主目录，覆盖 `SSHM_HOME`、密钥导入、Deploy 文件和 SSH config 导入导出等路径入口。
- 根命令帮助、补全候选和旧版 `--xxx` 选项建议改为共享同一份命令目录，减少帮助文案和补全候选漂移。
- 根命令帮助按“常用入口、主机与标签、连接、执行与传输、凭据与密钥、配置、日志与工具”分组。
- `config edit` 能正确解析带引号和空格的编辑器路径，例如 Windows 下的 `"C:\Program Files\...\editor.exe" --wait`。
- `README` 增加临时 `GOPROXY` 安装示例，便于网络访问 `proxy.golang.org` 不稳定时安装。

## 修复

- `delete` 支持 `--yes`，非交互环境未显式传入 `--yes` 时会直接报错，不再依赖空输入取消。
- 单主机 `exec` 默认写入操作日志，`--quiet` 只控制终端输出；需要关闭日志时使用 `--no-log`。
- Windows 测试不再假设 Unix 权限位语义，也不再硬编码 `/` 路径分隔符。

## 升级注意

- 主配置和 Deploy 配置仍为 `version: 2`。
- 从 v6.0.1 升级到 v6.0.2 不需要修改配置文件。

## 验收

```bash
go test ./...
go vet ./...
go test -race ./...
```

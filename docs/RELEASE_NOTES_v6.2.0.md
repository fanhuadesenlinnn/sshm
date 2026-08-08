# sshmd v6.2.0

v6.2.0 是产品改名版本：软件从 sshm 更名为 **sshmd**（Go 仓库同步改名）。主配置 schema 仍为 `version: 2`，Deploy schema 为 `version: 3`，配置与编排内容无需迁移。

## 改名

- Go 模块路径：`github.com/fanhuadesenlinnn/sshmd/v6`；`go install` 与 ldflags 同步更新。
- 二进制与命令：`sshmd`。
- 数据目录与主配置：默认 `~/.sshmd`、`sshmd.yaml`。
- 环境变量：`SSHMD_HOME`、`SSHMD_MASTER_PASSWORD`、`SSHMD_BECOME_PASSWORD`。
- 发布制品与安装脚本：`sshmd_*.tar.gz`，安装器目标二进制 `sshmd`。
- README、CONTEXT、PRODUCT_DESIGN、ROADMAP、初始化模板与全部用户可见文本统一为 sshmd。

## 升级注意

- 旧数据目录 `~/.sshm` 不会自动迁移；如需保留历史数据，请将其复制到 `~/.sshmd` 或设置 `SSHMD_HOME` 指向原目录。
- 历史档案（`docs/archive`、发布说明、ADR）保留原名，仅作记录。

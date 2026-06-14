# sshm v4.0.1

- 修复通过 `go install github.com/fanhuadesenlinnn/sshm/v4@latest` 安装后版本显示为 `dev` 的问题。
- 发布制品继续使用标签注入版本；普通 Go 安装从模块构建信息读取真实版本。

# sshm v2.3.1

## Linux 兼容性修复

- Linux amd64 和 arm64 制品改为纯 Go 静态构建。
- Linux 制品不再依赖目标机器的 glibc，可运行于 Alpine Linux 等 musl 环境。
- 发布流程会自动拒绝动态链接的 Linux 二进制，避免兼容性问题再次出现。
- 新增 CentOS 7（glibc 2.17）与 Alpine Linux（musl）容器启动验证。

## 升级说明

- 仅修复发布制品的构建方式，配置格式和功能行为没有变化。
- 使用旧版 Linux、Alpine 或遇到 `GLIBC_x.xx not found` 的用户应升级到此版本。

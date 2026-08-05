package config

// defaultDeployConfig is deliberately valid but contains no active profiles.
// The commented example helps first-time users without exposing them to an
// executable placeholder workflow after `sshm init`.
const defaultDeployConfig = `# sshm Deploy v2 编排配置
#
# 快速开始：
#   1. 添加主机并设置标签：sshm add web01 root@10.0.0.11 --tags prod,web
#   2. 编辑本文件，复制并取消注释下面的 profile 示例
#   3. 严格校验配置：      sshm deploy validate
#   4. 查看执行计划：      sshm deploy plan update-app
#   5. 模拟执行并看差异：  sshm deploy run update-app --check --diff
#   6. 确认无误后执行：    sshm deploy run update-app
#
# 默认读取 <SSHM_HOME>/deploy.yaml 和按文件名排序的 deploy.d/*.yaml。
# 使用 -f/--file 时只读取显式指定的文件，当前目录文件不会被自动加载。
#
# targets 可以使用 hosts、tags 或 all，三者选择一种；多个 tags 表示同时匹配。
# serial 控制每批主机数，parallel 控制批内并发数。
# 每个 step/handler 必须且只能包含一个动作：
# exec、push、pull、mkdir、wait 或 confirm。
# notify 在步骤发生变化后触发同名 handler。
#
# Deploy 只引用 sshm.yaml 中的主机和标签。
# 请勿在本文件中保存密码、私钥、SSH 用户、端口或主机信任信息。

version: 2

# 部署工作流列表；复制文件末尾的示例后再开始使用。
profiles: []

# 被 profile 步骤通过 notify 触发的处理器。
handlers: []

# 完整示例（全部为注释，不会被执行）：
# profiles:
#   - name: update-app
#     description: 更新应用并重启服务
#     targets:
#       tags: [prod, web]
#     serial: 2
#     parallel: 2
#     steps:
#       - name: 确认发布
#         confirm: 确认向生产环境发布？
#       - name: 创建发布目录
#         mkdir:
#           path: /opt/app/releases
#           mode: "0755"
#       - name: 上传应用包
#         push:
#           src: ./dist/app.tar.gz
#           dest: /opt/app/releases/app.tar.gz
#           checksum: true
#           backup: true
#         notify: [重启应用]
#       - name: 校验配置
#         exec: /opt/app/bin/check-config
#         check_safe: true
#       - name: 等待服务稳定
#         wait: 3s
#
# handlers:
#   - name: 重启应用
#     exec: systemctl restart app
#     become: true
#     become_user: root
`

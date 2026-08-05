package deploy

const Sample = `# sshm Deploy v2 可运行示例
#
# 使用前请先修改 targets、路径和命令，然后依次运行：
#   sshm deploy validate
#   sshm deploy plan update-app
#   sshm deploy run update-app --check --diff
#   sshm deploy run update-app
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

# 工作流列表；name 在所有加载的 Deploy 文件中必须唯一。
profiles:
  - name: update-app
    description: 更新应用并重启服务
    # 选择同时带有 prod 标签的主机。
    targets:
      tags: [prod]
    # 每批 2 台主机，每批最多并发执行 2 台。
    serial: 2
    parallel: 2
    steps:
      - name: 确认发布
        confirm: 确认向生产环境发布？
      - name: 创建发布目录
        mkdir:
          path: /opt/app/releases
          mode: "0755"
      - name: 上传应用包
        push:
          src: ./dist/app.tar.gz
          dest: /opt/app/releases/app.tar.gz
          checksum: true
          backup: true
        notify: [重启应用]
      - name: 校验配置
        exec: /opt/app/bin/check-config
        check_safe: true
        changed_when:
          rc_in: [10]
      - name: 等待服务稳定
        wait: 3s

# handler 仅在同名 notify 被已变化步骤触发后运行。
handlers:
  - name: 重启应用
    exec: systemctl restart app
    become: true
    become_user: root
`

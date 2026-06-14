package deploy

const Sample = `# sshm deploy v2 配置示例
# 修改目标、路径和命令后，先运行 sshm deploy validate / plan
version: 2

profiles:
  - name: update-app
    description: 更新应用并重启服务
    targets:
      tags: [prod]
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

handlers:
  - name: 重启应用
    exec: systemctl restart app
    become: true
    become_user: root
`

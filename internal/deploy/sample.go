package deploy

const Sample = `# sshm deploy 配置；只描述目标、步骤与策略，不保存 SSH 凭据
# 修改目标、路径和命令后，先运行 sshm deploy validate / plan
version: 1
name: project-operations

vars:
  package_dir: ./dist/app
  app_dir: /opt/app

defaults:
  strategy:
    mode: hidden
    max_parallel: 5
    connect_timeout: 30s
    step_timeout: 15m
    retry_count: 0
    retry_on_stage: [network, transfer]

profiles:
  - name: install
    description: upload package and run installer
    targets:
      tags: [prod]
    steps:
      - name: upload
        type: copy
        src: ${package_dir}
        dest: ${app_dir}
        method: auto
        overwrite: true
      - name: install
        type: exec
        command: bash ${app_dir}/install.sh
`

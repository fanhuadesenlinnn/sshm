package config

// DemoDeployV3 is the active, non-mutating Deploy v3 demo created by
// `sshmd deploy init --dir`. Unlike SampleDeployV3, this demo is immediately
// plannable once a host tagged prod exists.
const DemoDeployV3 = `# sshmd Deploy v3 安全 demo
#
# 本示例只包含 debug 与 include；--check 不会修改远端。
# 主机、密码和私钥仍由 sshmd 主配置管理，请勿写入本文件。

version: 3

plays:
  - name: update-app
    description: 安全演示 vars_files、debug、include 与 --check
    hosts:
      tags: [prod]
    vars_files:
      - ./vars/versions.yaml
    tasks:
      - name: 显示演示目标
        debug:
          msg: "准备检查 {{ env }} 环境（标签 {{ tags }}）"
      - name: 加载安全检查片段
        include: ./tasks/prepare.yaml
`

// DemoTasksFile is the safe example include fragment created by
// `sshmd deploy init --dir`, referenced from DemoDeployV3.
const DemoTasksFile = `tasks:
  - name: include 中的只读检查
    debug:
      msg: "{{ env }} 环境已进入安全演示；未执行任何写操作"
`

// DemoVarsFile is the example vars file created by `sshmd deploy init --dir`.
const DemoVarsFile = `# 示例变量文件：由 deploy.yaml 的 vars_files 引用，路径相对 deploy.yaml。
# play 内 vars 与 CLI --extra-var 可以覆盖这里的值。
env: "prod"
tags: "prod"
`

// DemoReadme is the short guide written into a demo directory.
const DemoReadme = `# Deploy demo

本目录是由 sshmd deploy init --dir 生成的安全编排示例。它只使用 debug、include 和 --check，不会修改远端。

先确保至少有一台带 prod 标签的主机：

    sshmd add web01 root@10.0.0.11 --tags prod

然后校验并查看执行计划：

    sshmd deploy validate -f ./deploy.yaml
    sshmd deploy plan update-app -f ./deploy.yaml
    sshmd deploy run update-app -f ./deploy.yaml --check --diff --yes

## 文件

| 路径 | 作用 |
| --- | --- |
| deploy.yaml | 含 update-app play 的可运行安全示例 |
| tasks/prepare.yaml | 只含 debug 的 include 示例片段 |
| vars/versions.yaml | vars_files 示例变量 |

编排只引用主配置中的主机与标签；请勿在本目录保存密码或私钥。需要编写真实发布流程时，另建 play 并先用 --check 检查计划。
`

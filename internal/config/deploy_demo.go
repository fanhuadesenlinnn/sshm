package config

// DemoTasksFile is the example include fragment created by
// `sshmd deploy init --dir`, referenced from the commented update-app example.
const DemoTasksFile = `tasks:
  - name: 公共准备
    file:
      path: /tmp/prep
      state: directory
`

// DemoVarsFile is the example vars file created by `sshmd deploy init --dir`.
const DemoVarsFile = `# 示例变量文件：由 deploy.yaml 的 vars_files 引用，路径相对 deploy.yaml。
# play 内 vars 与 CLI --extra-var 可以覆盖这里的值。
env: "prod"
tags: "core"
`

// DemoReadme is the short guide written into a demo directory.
const DemoReadme = `# Deploy demo

本目录是由 sshmd deploy init --dir 生成的编排示例，自包含且可直接校验：

    sshmd deploy validate -f ./deploy.yaml
    sshmd deploy plan update-app -f ./deploy.yaml
    sshmd deploy run update-app -f ./deploy.yaml --check --diff --yes

## 文件

| 路径 | 作用 |
| --- | --- |
| deploy.yaml | Deploy v3 完整注释模板（plays 为空，示例全部注释） |
| templates/app.conf.tmpl | template 模块示例模板 |
| tasks/prepare.yaml | include 示例片段 |
| vars/versions.yaml | vars_files 示例变量 |

把 deploy.yaml 末尾注释示例复制到 plays 下并修改目标主机后即可运行。编排只引用主配置中的主机与标签；请勿在本目录保存密码或私钥。
`

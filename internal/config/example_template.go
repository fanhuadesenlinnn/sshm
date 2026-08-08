package config

// ExampleTemplateFile is a commented template example created in
// <SSHMD_HOME>/templates/ by sshmd init. It is referenced by the commented
// update-app example in deploy.yaml.
const ExampleTemplateFile = `# 模板示例：由 template 模块渲染后上传到远端。
# {{ }} 中引用 deploy.yaml 里的变量；| default 提供变量缺失时的兜底值。
# 渲染结果会与远端现有文件比对，内容一致时判定为 ok（幂等）。
server_name={{ server_name | default "app.local" }}
version={{ app_version }}
port={{ port | default 8080 }}
`

package config

// DefaultREADME is a one-page quickstart written into the sshmd home.
const DefaultREADME = `# sshmd 快速上手

本目录是 sshmd 的全部本地状态（SSHMD_HOME）。所有文件都可放心阅读；密码和私钥只保存在加密 vault 中，不会出现在文件里。

## 文件说明

| 路径 | 作用 |
| --- | --- |
| sshmd.yaml | 主配置：默认值、主机、标签、托管密钥、主机信任、加密 vault |
| deploy.yaml | Deploy v3 编排入口，含完整注释示例 |
| deploy.d/ | 可选的编排分片目录，按文件名排序合并 |
| templates/ | template 模块的模板文件目录 |
| logs/ | 操作日志（默认保留 30 天） |
| backups/ | 覆盖配置前的备份 |
| tmp/ | 传输临时文件 |

## 常用命令

    sshmd                              # 打开轻量工作台
    sshmd add web01 root@10.0.0.11     # 添加主机
    sshmd ping web01                   # 测试连接
    sshmd exec web01 "uptime"          # 在单台主机执行命令
    sshmd exec-tag prod "uptime"       # 在标签主机批量执行（all 是虚拟全量标签）
    sshmd passwd web01                 # 加密保存 SSH 密码
    sshmd push web01 ./file /tmp/file  # 上传文件
    sshmd pull web01 /tmp/file ./file  # 下载文件
    sshmd deploy validate              # 校验编排配置
    sshmd deploy plan update-app       # 查看执行计划
    sshmd deploy run update-app --check --yes   # 模拟执行
    sshmd deploy run update-app --yes           # 真正执行

## 安全边界（请牢记）

- --yes 只跳过执行确认，不跳过主密码和主机信任
- 主机信任策略默认 strict，首次连接需要确认
- 编排文件里不要放密码、私钥、SSH 用户或端口
- --all 不能与具体主机或标签混用，避免误操作

## 下一步

    sshmd add web01 root@10.0.0.11 --tags prod
    sshmd deploy validate
    sshmd doctor
`

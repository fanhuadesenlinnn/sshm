package config

// DefaultREADME is the comprehensive guide written into the sshmd home at
// initialization. It covers the data layout, host management, credentials,
// transfers, Deploy orchestration, logging and security so a new user can
// learn the tool from the generated files alone.
const DefaultREADME = `# sshmd 使用指南

sshmd 是一个本地优先、面向个人使用的 SSH 主机管理与轻量运维工具。它管理主机、标签、凭据、批量命令、安全文件传输与 Deploy 编排。本目录（SSHMD_HOME，默认 ~/.sshmd）是 sshmd 的全部本地状态。

## 目录与文件

| 路径 | 作用 |
| --- | --- |
| sshmd.yaml | 主配置：默认值、主机、标签、托管密钥、主机信任、加密 vault |
| deploy.yaml | Deploy v3 编排入口，含完整注释示例 |
| deploy.d/ | 可选编排分片目录，按文件名排序合并 |
| templates/ | template 模块的模板文件目录 |
| facts/ | gather_facts 的主机事实缓存 |
| logs/ | 操作日志（默认保留 30 天） |
| backups/ | 覆盖配置前的备份 |
| tmp/ | 传输临时文件 |

所有文件均为当前用户私有（0700/0600）。密码与私钥默认只保存在加密 vault 中。

## 主机管理

    sshmd add web01 root@10.0.0.11                  # 添加主机（默认 root/22）
    sshmd add web01 root@10.0.0.11 --tags prod,web   # 添加并打标签
    sshmd add web01 root@10.0.0.11 --password-stdin  # 终端隐藏输入；不会暴露到 argv
    sshmd add-batch "web01=root@10.0.0.11" "db01=root@10.0.0.12"
    sshmd list                                       # 列出主机（ls/l 亦可）
    sshmd search prod                                # 按关键词搜索
    sshmd show web01                                 # 查看主机详情
    sshmd edit web01                                 # 交互编辑
    sshmd delete web01 --yes                         # 删除
    sshmd tag                                        # 标签管理
    sshmd pin web01 / sshmd recent                   # 收藏 / 最近使用
    sshmd find-con                                   # 可搜索主机选择器（f/pick）

主机由稳定的内部 id 关联凭据；手工编辑 sshmd.yaml 新增主机时可省略 id，校验后自动生成。别名只能包含字母、数字、点、下划线和短横线。

## 认证与凭据

密码有两种存储方式，可并存：

- 加密 vault（推荐）：主密码保护，写入 sshmd.yaml 的加密区。
- 明文 password 字段（便捷）：直接写在主机条目里，受 0600 权限保护；sshmd doctor 会提醒。可用 sshmd passwd 一键升级为 vault 加密并清掉明文字段。

add 的 --password-stdin 会在终端隐藏输入，也支持从管道读取；它只避免密码进入进程参数，仍按兼容模式写入明文 password 字段。需要加密保存时优先先 add，再运行 sshmd passwd。

    sshmd passwd web01                  # 加密保存密码（对明文主机自动升级）
    sshmd passwd --tag prod             # 批量保存
    sshmd forget-pass web01             # 删除保存的密码
    sshmd auth web01                    # 修改认证策略（auto/key/password）
    sshmd key create mykey --default    # 生成托管密钥
    sshmd key push mykey web01 --yes    # 推送公钥到远端
    sshmd key setup mykey web01 --yes   # 推送、验证并绑定
    sshmd key status                    # 查看绑定状态
    sshmd lock                          # 锁定当前会话密码库

主机信任策略：strict（默认，首次连接确认、密钥变化拒绝）、accept-new（自动信任新主机）、insecure（跳过校验，不推荐）。主密码只在当前进程内按需解锁。

## 连接与执行

    sshmd web01                          # 直连主机（也支持 ID）
    sshmd ping web01                     # 测试连接
    sshmd exec web01 "uptime"            # 执行命令
    sshmd exec web01 -- "systemctl status nginx"
    sshmd exec-tag prod "uptime" --yes   # 按标签批量执行（all 是虚拟全量标签）
    sshmd exec-tag prod --exclude db01 "date"

批量选项：--serial N（每批台数）、--parallel N（批内并发）、--fail-fast、--max-fail N、--max-fail-percent N、--exclude/--exclude-tag、--yes（跳过确认）。

退出码：0 成功；1 失败或失败策略产生跳过；2 主机不可达；3 参数或配置错误；4 执行前 vault/认证阻断；130 用户中断。

## 文件传输

    sshmd push web01 ./app.tar.gz /opt/app.tar.gz    # 上传
    sshmd pull web01 /var/log/app.log ./logs/        # 下载
    sshmd push-tag prod ./conf /etc/app/conf         # 批量上传
    sshmd pull-tag prod /var/log ./logs/             # 批量下载

默认 SHA-256 校验；目标已存在且内容不同时会拒绝，需显式 --overwrite 或 --backup。目录传输逐文件 manifest 校验；符号链接与特殊文件被拒绝。多主机 pull 会预先检测目标路径冲突。

## 端口转发

    sshmd forward web01 127.0.0.1:8080 127.0.0.1:80

把远端 80 端口临时映射到本地 8080，Ctrl+C 停止。

## Deploy 编排

Deploy 是 v3 模块化 playbook：plays（工作流）包含 hosts、strategy、批量策略、vars 与 tasks；每个 task 调用一个幂等模块。文件默认在 deploy.yaml 与 deploy.d/，项目文件用 --file 显式指定。

    sshmd deploy init                 # 生成带注释的示例
    sshmd deploy validate             # 严格校验
    sshmd deploy list                 # 列出 plays
    sshmd deploy plan update-app      # 查看执行计划（不连远端）
    sshmd deploy run update-app --check --diff --yes   # 模拟执行看差异
    sshmd deploy run update-app --yes                 # 真正执行
    sshmd deploy run update-app --output ndjson --yes # 事件流输出

模块（13 个）：command（静态 cmd 按字面 argv 解析；模板值使用 argv 列表逐元素安全引用）、shell、file（directory/file/link/absent + mode/owner）、copy（src 或 content + backup）、template（{{ }} 插值渲染）、service（systemctl 状态机）、wait_for（path/port，connect_from: controller|target）、sleep（定时延时，check 自动跳过）、unarchive（拒绝路径穿越、链接、特殊文件和解压炸弹）、fetch（远端拉取到本地）、pause（人工暂停）、fail（安全闸门）、debug。

任务特性：register/when（is defined、in、not in）、loop、run_once、become/become_user、ignore_errors、failed_when/changed_when、check_safe、confirm（linear 策略每个 serial 批次前确认）、env、block/rescue/always、include（静态片段）、vars_files、strategy linear|free、gather_facts。

become 提权无需免密 sudo：密码只可来自环境变量 SSHMD_BECOME_PASSWORD，或自动复用该主机 vault 中的 SSH 密码；密码经 stdin 传给 sudo -S，不落日志。Deploy 文件中的 become_password 会被严格拒绝。

一个最小 play 示例（放在 deploy.yaml 的 plays 下）：

    - name: update-app
      hosts:
        tags: [prod]
      strategy: linear
      serial: 2
      tasks:
        - name: 创建目录
          file:
            path: /opt/app/releases
            state: directory
            mode: "0755"
        - name: 上传应用包
          copy:
            src: ./dist/app.tar.gz
            dest: /opt/app/releases/app.tar.gz
          register: upload
        - name: 重启服务
          service:
            name: app
            state: restarted
          when: upload.changed
          become: true

## 日志与诊断

    sshmd doctor                       # 环境自检（配置、凭据、Deploy）
    sshmd config path                  # 查看当前路径
    sshmd config show                  # 查看主配置
    sshmd logs                         # 查看操作日志
    sshmd logs --host web01            # 按主机过滤
    sshmd logs --action deploy         # 按动作过滤
    sshmd logs clean --yes             # 清理日志
    sshmd export-ssh-config ./cfg      # 导出 OpenSSH 配置

## 安全边界

- --yes 只跳过当前操作确认，不跳过主密码和主机信任。
- 主机信任默认 strict；主机密钥变化会被拒绝。
- 密码默认进加密 vault；明文 password 字段是可选的便捷方式，受 0600 保护。
- Deploy 编排文件里不要放密码、私钥、SSH 用户或端口。
- --all 不能与具体主机或 --tag 混用，避免意外扩大操作范围。
- 日志可能包含远程输出，请按本地敏感数据对待。

## 快速上手

    sshmd add web01 root@10.0.0.11 --tags prod --host-key-policy accept-new
    sshmd passwd web01
    sshmd ping web01
    sshmd exec web01 "uptime"
    sshmd push web01 ./app.tar.gz /opt/app.tar.gz
    sshmd deploy validate
    sshmd deploy run update-app --check --diff --yes

更多帮助：sshmd --help、sshmd deploy --help、sshmd key --help、sshmd tag --help。deploy.yaml 顶部的注释是完整的编排参考。
`

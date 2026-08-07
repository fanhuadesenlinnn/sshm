package config

// SampleDeployV3 is the commented Deploy v3 starter manifest. It is
// deliberately valid YAML with no active plays; every example is commented so
// `sshm deploy validate` passes immediately after `sshm init`.
const SampleDeployV3 = `# sshm Deploy v3 编排配置
#
# 快速开始：
#   1. 添加主机：sshm add web01 root@10.0.0.11 --tags prod
#   2. 阅读本文件（全部是注释，不会执行），把末尾的示例复制到 plays 下并按需修改
#   3. 校验配置：sshm deploy validate
#   4. 查看计划：sshm deploy plan update-app
#   5. 模拟执行：sshm deploy run update-app --check
#   6. 确认无误后执行：sshm deploy run update-app --yes
#
# 默认读取 <SSHM_HOME>/deploy.yaml 和按文件名排序的 deploy.d/*.yaml。
# 使用 -f/--file 时只读取显式指定的文件，当前目录文件不会被自动加载。
#
# v3 模型：plays（工作流）-> tasks（任务）-> modules（模块，幂等）。
#
# 安全边界：
#   - 本文件只描述"做什么"，请勿在本文件中保存密码、私钥、SSH 用户、端口或主机信任
#   - 密钥一律由主配置的 vault 管理（sshm passwd / sshm key）
#   - 需要人工确认的操作用 pause 模块表达

version: 3

# 全局变量；play 内 vars、vars_files、CLI --extra-var 逐级覆盖。
# 引用方式 {{ 变量名 }}，支持 default/join/upper/lower/trim/replace 等函数：
#   {{ app_version }}
#   {{ port | default 8080 }}         # 变量缺失时使用默认值
vars:
  app_version: "1.4.2"

# 工作流列表；play 名在所有加载文件中必须唯一。
plays: []

# ==================== 完整示例（全部为注释，复制到 plays 下即可运行）====================
#
# plays:
#   - name: update-app
#     description: 更新应用并重启服务
#     hosts:
#       tags: [prod]          # 三种目标选择，三选一：hosts 列表 / tags 标签 / all
#     strategy: linear        # linear=所有主机完成当前任务才进下一步；free=按主机跑完
#     serial: 2               # 每批几台主机，0=一批全跑；滚动发布常用 1-2
#     parallel: 4             # 批内最多几台并发（1-128）
#     timeout: 30s            # 单个任务最长执行时间（可选）
#     connect_timeout: 10s    # 单台主机连接超时（可选）
#     fail_fast: false        # 任一主机失败立即停止调度后续主机
#     max_fail: 0             # 失败 N 台后停止（0=不限）
#     gather_facts: true      # 收集主机事实：hostname/system/arch/os_family
#     vars:                   # play 级变量，覆盖文件级 vars
#       base: /opt/app
#     vars_files:             # 额外变量文件（可选），路径相对本文件
#       - ./vars/versions.yaml
#     tasks:
#       - name: 创建目录
#         file:
#           path: "{{ base }}/releases"
#           state: directory          # directory | file | link | absent
#           mode: "0755"
#       - name: 上传应用包
#         copy:
#           src: ./dist/app-{{ app_version }}.tar.gz   # 相对本文件所在目录
#           dest: "{{ base }}/app.tar.gz"
#           backup: true
#         register: upload            # 结果保存为 upload，供后续 when 判断
#       - name: 渲染配置
#         template:
#           src: ./templates/app.conf.tmpl              # 模板文件，{{ }} 插值
#           dest: /etc/app.conf
#         when: upload.changed        # 只有真的变了才执行（register + when 取代 handlers）
#       - name: 重启服务
#         service:
#           name: app
#           state: restarted          # started | stopped | restarted | enabled | disabled
#         become: true                # 需要 root 时开启（要求目标机配置免密 sudo）
#       - name: 等待端口就绪
#         wait_for:
#           port: 8080
#           timeout: 30s
#       - name: 等待应用完全启动
#         sleep:
#           seconds: 5               # 纯延时；也支持 duration: 5s。check 模式自动跳过
#       - name: 每批主机执行前人工确认
#         command:
#           cmd: /opt/app/bin/restart
#         confirm: 确认重启这组主机?    # linear 策略下每个 serial 批次开始前确认
#       - name: 发布失败自动回滚
#         block:                      # 一组任务；失败时执行 rescue，无论成败执行 always
#           - command:
#               cmd: /opt/app/bin/check
#         rescue:
#           - command:
#               cmd: /opt/app/bin/rollback
#         always:
#           - debug:
#               msg: "发布流程结束，版本 {{ app_version }}"
#       - name: 人工门禁
#         pause:
#           message: 确认发布到生产?
#
# ==================== 其他模块简注 ====================
#   command：不经过 shell，cmd 不能含管道/重定向/$/;/&/反引号；shell：完整 shell 语法
#     command:
#       cmd: "ls -la {{ base }}"
#       chdir: /opt/app        # 先切换目录
#       creates: /opt/app/installed   # 路径存在则跳过（幂等钩子）
#       removes: /opt/app/legacy      # 路径不存在则跳过
#   unarchive：上传并解压压缩包（.tar.gz/.tgz/.zip，内置路径穿越校验）
#     unarchive:
#       src: ./dist/bundle.tar.gz
#       dest: "{{ base }}/bundle"
#   fetch：从远端拉文件到本地（dest 相对本文件目录；flat=true 直接放 dest 下）
#     fetch:
#       src: /var/log/app.log
#       dest: ./logs/
#       flat: true
#   fail：安全闸门，配合 when 阻止继续
#     fail:
#       msg: 环境校验未通过
#     when: os_family == 'debian'      # when 支持 facts（需 gather_facts: true）
# when 语法：== != < <= > >=、&& || !（及 and/or/not）、in/not in、
#           is defined/is not defined、括号、数字/字符串/true/false/null 字面量。
# 引用未定义变量会直接报错（而不是静默跳过）；可选变量请先写 x is defined and ... 再引用。
# loop 任务中 when 按每个 item 求值，可用 when: item != 'x' 跳过单项。
#
# ==================== 任务片段复用（可选）====================
# 公共任务可以放到单独文件，用 include 静态引入（带循环检测）：
#   tasks:
#     - include: ./tasks/prepare.yaml
# 片段文件内容示例：
#   tasks:
#     - name: 公共准备
#       file:
#         path: /tmp/prep
#         state: directory
`

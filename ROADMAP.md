# sshmd Roadmap

本文档记录候选能力，不是当前版本承诺。

## 候选项

- 批量目标排除：已落地（`--exclude`/`--exclude-tag`，exec-tag/push-tag/pull-tag 与 deploy 目标覆盖）。
- Deploy 简单变量替换的正式契约：已在 v3 落地（`{{ }}` 插值、vars/vars_files/`--extra-var`、模板模块）。
- 更丰富的 BatchRunner 机器可读输出：已在 v3 落地（`--output ndjson` 事件流）。
- Deploy v3 模块化 playbook：已落地（plays/tasks/modules、register/when、block/rescue/always、静态 include、sleep/confirm）。
- Deploy `--force-handlers`：已过时——v3 移除 handlers，条件执行使用 register + when。
- 更精细的文本 unified diff 与可配置 diff 日志策略。
- rsync 跨平台高保真集成测试环境，覆盖真实远端 `rsync --server` 行为。
- Windows 路径与发布制品的持续兼容性测试扩展。

## 持续非目标

- 团队账号、共享凭据、集中权限审批。
- 后台调度、守护进程、持续部署服务。
- 完整 Ansible 兼容层、roles、facts、循环与期望状态收敛。
- 任意 OpenSSH 参数透传。

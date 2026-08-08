# sshmd v6.2.2

v6.2.2 新增 `sshmd deploy init --dir`：把整套可校验的 Deploy demo 初始化进任意目录，便于项目脚手架与教学演示。主配置 schema 与 Deploy schema 均无变化。

## 新功能

- `sshmd deploy init --dir <目录>` 生成自包含 demo：deploy.yaml（完整注释模板）、templates/app.conf.tmpl、tasks/prepare.yaml、vars/versions.yaml、README.md。
- 相对路径闭环：模板示例中的 template/include/vars_files 引用都指向生成目录内文件，展开示例后 `validate`/`plan`/`run` 直接可用。
- 与 `-f`/`--stdout` 互斥；目录自动创建，已存在文件默认跳过，`--overwrite` 覆盖。

## 质量

- 新增生成完整性（文件齐全 + validate 通过）与跳过/覆盖语义的回归测试，全量测试通过。

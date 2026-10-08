<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-08 11:48 / 结束 2026-10-08 11:53 / 规范版本 @sw/dev-skills@1.1.2 -->

# 实现计划

## 背景

任务类型：bugfix。修正服务器迁移后的发布目标、SSH 端口和源码更新说明。

## 关键文件与分层

| 文件 | 改动 | 调用方与影响 |
|---|---|---|
| `DEPLOYMENT.md` | 新服务器信息、命令、迁移配置说明 | 人工运维、两个发布技能、README/贡献指南 |
| `.codex/skills/quick-release/SKILL.md`、`.claude/skills/quick-release/SKILL.md` | SSH/rsync 目标、生产 Compose 文件参数 | Codex/Claude 发布入口 |
| `docker-compose.production.yml` | 新增显式生产覆盖文件 | 当前迁移服务器更新时选择，基础配置保持首次部署可用 |
| `deploy.sh` | 新访问地址提示 | 首次部署完成后的终端提示 |
| `README.md`、`CONTRIBUTING.md` | 生产入口与迁移注意事项 | 项目使用者与贡献者 |
| `.gitignore`、`.dockerignore` | 排除服务器备份、迁移 JSON；构建上下文同时排除 `.env` | Git 与生产镜像构建，避免数据库和解析后的凭据被收集 |

`codegraph callers deploy.sh` 与 `codegraph callees deploy.sh` 均返回 `ℹ Symbol "deploy.sh" not found`。通过文本检索补齐发布技能和文档引用链。本次属于项目运维配置，不触及跨客户公共业务代码；不要求新增单元测试。

## 验证前置

1. 从文档中的 `ssh -p 7339 root@123.56.219.4` 发起连接，读取用户与部署目录；再执行文档同步命令的 `rsync --dry-run` 版本，预期认证成功、同步目标为现有部署目录，退出码为 0，不上传文件。
2. `bash -n deploy.sh`、`git diff --check` 通过；两份 skill 逐字一致，并通过 `quick_validate.py`。
3. 全仓库当前部署说明不再引用迁移前地址；所有远程发布命令使用新 IP 与端口。
4. 在服务器通过临时文件解析仓库基础 Compose 与生产覆盖文件（`docker compose config`，不执行 up/build），预期后端保留 `build`，项目名、端口和 external 卷名与现状一致。临时文件自动清理，环境值不输出。
5. 最终暂存范围只包含本次配置、文档和阶段记录；不包括 `.env`、SSH 文件、迁移 JSON、版本或业务代码。
   备份和迁移 JSON 的路径应被 Git 与构建上下文排除；备份写入服务器 `backups/`，恢复从同一目录读取，不依赖原容器 `/tmp`。
6. 独立子智能体复核通过后提交推送；用远端 main SHA 与本地 HEAD 相等确认推送结果。

受影响测试：`codegraph affected deploy.sh` 原始输出为 `ℹ No test files affected by the changed files.`。项目有 Go 测试，前端未配置测试命令，本次没有业务逻辑改动，不运行业务测试或启动开发服务。

## 改动清单

- [x] `docker-compose.production.yml` —— 固定生产项目与 external 卷名 —— 服务器只解析配置并核对现有挂载。
- [x] `DEPLOYMENT.md` —— 更新服务器、端口、运维命令与迁移配置说明 —— 核对 SSH/rsync 与配置参数，区分已验证事实。
- [x] `.codex/skills/quick-release/SKILL.md`、`.claude/skills/quick-release/SKILL.md` —— 同步发布与备份命令 —— 逐字一致、技能校验通过。
- [x] `deploy.sh` —— 更新完成提示中的访问地址 —— Shell 语法检查通过。
- [x] `README.md`、`CONTRIBUTING.md` —— 明确当前生产入口、迁移凭据和数据注意事项 —— 核对链接与事实。
- [x] `.gitignore`、`.dockerignore` —— 排除备份、迁移 JSON 与构建凭据 —— `git check-ignore` 和忽略规则核对。
- [x] `docs/ai/更新生产服务器配置/03-review.md` —— 独立验收并记录实际验证与限制 —— 无阻塞问题后提交推送。

## 范围与风险

本次不升级版本、不发版、不部署、不修改服务器运行配置/数据/凭据，不改业务代码。安全组全部规则未查询；构建、实际发布和数据完整性验收留待用户明确上线时执行。external 生产覆盖文件只用于已有指定数据卷的服务器，首次部署使用基础文件。

## 实现验证记录

- `bash -n deploy.sh`、`git diff --check`、两个技能的 `cmp` 均通过；所有文档/技能 SSH 与 rsync 命令已核对 IP 和端口。
- 服务器临时解析两份仓库 Compose：项目名、`backend.build`、39100→8080、external 卷与现有卷一致；临时目录已清理，服务器原 Compose 哈希未变。
- 两份技能通过原版 `quick_validate.py`。本机 Python 缺少 PyYAML，改在服务器已有 PyYAML 环境的临时目录执行同一校验器，未安装依赖或修改运行配置。
- rsync `--dry-run` 退出码 0，确认需要同步基础与生产配置，未上传文件。首次捕获输出遇到 UTF-8 解码错误，改按字节捕获后再次 dry-run 成功；两次均未实际同步。
- 仓库没有旧生产地址引用；未修改版本、业务代码、服务器凭据或数据。
- 独立复核发现原容器 `/tmp` 备份不能保证重建后保留；已纳入本次运维文档修正范围，改为服务器持久化目录并补排除规则。
- 更新后的三个备份命令均通过 `bash -n`，输出由远端 shell 写入服务器 `backups/`，密码只在容器内展开；未实际执行备份或恢复。
- `.env`、`backups/`、`compose.migrated.json` 的 Git 忽略检查通过，构建上下文具备同样排除规则；两个更新后的技能再次校验通过。
- 备份命令增加 `set -C` 与 `test -s`，阻止同名覆盖并检查文件非空；rsync 同时排除备份目录与迁移 JSON，避免忽略文件被同步覆盖。

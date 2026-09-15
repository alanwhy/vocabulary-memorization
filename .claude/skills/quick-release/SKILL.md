---
name: quick-release
description: 当用户以“快速上线”“快速发布”“一键上线”“发布上线”或“上线部署”等明确指令要求发布当前改动时，完成本项目的版本更新、文档更新、Git 提交推送与远端生产部署。该流程不依赖本机 Docker，不在本地构建或验证镜像；仅适用于实际发布，不用于询问发布方法。
---

# 快速上线

此技能仅适用于当前 `vocabulary-memorization` 仓库。用户的明确上线指令即授权本次发布所需的版本更新、文档更新、提交、推送和生产部署；不要将“如何快速上线”“上线流程是什么”等咨询性提问视为发布授权。

## 发布边界

- 生产分支固定为 `main`，远端为 `origin/main`。只在本地已检出的 `main` 且与远端不存在待整合差异时发布；否则停止并说明原因，不创建分支、不合并、不拉取、不强推。
- 只提交本次发布范围内的已有代码改动和本技能要求更新的版本、文档。发现无关的未提交文件或不明来源的改动时，停止并列出它们，不能把它们一并带入发布。
- 不创建 Git tag，不发布 npm 包，不修改服务器 `.env`，不输出或收集密钥。
- 不运行本机 Docker、`docker build`、本地编译、测试或线上冒烟；镜像构建仅在第 5 步的远端 `docker compose up -d --build` 中完成。
- 部署失败时保留错误信息并停止；不得自行重试、回滚、改服务器配置或进行破坏性操作。

## 执行顺序

1. 阅读仓库 `AGENTS.md`、`DEPLOYMENT.md`，确认当前分支、工作区、上游状态和待发布 diff。远端未快进或工作区存在无关改动时停止。
2. 确定语义化版本号：用户指定版本时严格使用该版本；未指定时，纯缺陷修复升补丁号，新增向后兼容功能升次版本号，破坏性变更仅在用户明确要求时升主版本号。当前版本源为 `frontend/package.json`，使用 `npm version <版本号> --no-git-tag-version` 同步更新 `frontend/package-lock.json`，不得生成 Git tag。
3. 在 `CHANGELOG.md` 顶部写入对应版本和当天日期，仅根据当前待发布改动记录 `Added`、`Changed`、`Fixed`。仅在功能、配置、部署方式或使用说明确有变化时同步更新相关 README、部署文档和已有的 `docs/ai` 产物；不要为了凑发布而改无关文档。
4. 复核将暂存的文件和 diff，使用 `release: v<版本号>` 提交并推送 `origin main`。推送成功前不得部署。
5. 按 `DEPLOYMENT.md` 更新生产环境：
   - 若本次涉及数据库结构或数据迁移，先在服务器执行以版本号命名的 MySQL 全库备份；无法判断时按涉及迁移处理。
   - 用 `rsync -avz` 同步本仓库到 `root@101.42.45.60:/root/vocabulary-memorization/`，并排除 `.git`、`.env`、`.codegraph` 和 `node_modules`，保留服务器密钥与数据。
   - 通过 SSH 在服务器目录执行 `docker compose up -d --build`。这是更新部署，不要运行仅适用于首次部署、可能生成 `.env` 的 `deploy.sh`。
6. 报告版本号、提交 SHA、推送结果和部署命令结果；明确说明本技能未执行本机构建、测试、线上冒烟或功能验收。

## 数据库变更备份

部署前的备份使用服务器现有环境变量，密码不得打印到本机终端：

```bash
ssh root@101.42.45.60 "cd /root/vocabulary-memorization && docker compose exec -T mysql sh -c 'mysqldump -uroot -p\"\$MYSQL_ROOT_PASSWORD\" --all-databases > /tmp/backup_pre_<版本号>.sql'"
```

部署同步和重建命令如下；执行时将本地路径替换为当前仓库的绝对路径：

```bash
rsync -avz --exclude='.git' --exclude='.env' --exclude='.codegraph' --exclude='node_modules' \
  <本地仓库绝对路径>/ root@101.42.45.60:/root/vocabulary-memorization/
ssh root@101.42.45.60 "cd /root/vocabulary-memorization && docker compose up -d --build"
```

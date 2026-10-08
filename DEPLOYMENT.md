# 部署说明（当前实际部署逻辑）

记录当前生产服务器与更新流程。2026-10-08 已核对迁移后的 SSH、公网访问、容器状态和数据卷；实际镜像构建与发布仍按下列更新流程执行。

## 服务器信息

- 地址：`123.56.219.4`，用户：`root`，SSH 端口：`7339`（本机已配置公钥免密登录）
- 部署目录：`/root/vocabulary-memorization`
- 访问地址：`http://123.56.219.4:39100`
- 技术栈：Docker Compose 起两个容器 —— `mysql:8.0` + Go 后端（`backend/Dockerfile` 构建）
- Compose 项目名：`vocabulary-memorization`

SSH 命令显式带 `-p 7339`，rsync 显式带 `-e 'ssh -p 7339'`，使发布流程不依赖某台本机的 SSH 端口配置。服务器密码、私钥和 `.env` 内容不写入仓库。

## 迁移后的配置与数据卷

迁移时导入了现有镜像：服务器 `docker-compose.yml` 与 `compose.migrated.json` 内容一致，后端只有 `image`，没有 `build`；当前容器的启动配置记录为 `compose.migrated.json`。因此，只在这份迁移配置上执行 `up --build` 不会构建新源码。

后续更新先按下面的 rsync 流程同步仓库，将基础 `docker-compose.yml` 恢复为源码构建配置，再同时加载 `docker-compose.production.yml`。生产覆盖文件固定项目名，并将以下已有卷声明为 external：

| 数据 | 卷名 | 容器挂载 |
|---|---|---|
| MySQL | `vocabulary-memorization_mysql_data` | `/var/lib/mysql` |
| 音频 | `vocabulary-memorization_audio_data` | `/app/audio` |

两卷存在时复用；缺失时部署报错，避免误建空卷。首次在空服务器部署仍使用基础 `docker-compose.yml`，不加载生产覆盖文件。保留迁移 JSON 作为服务器上的记录，不复制到 Git，也不作为后续源码更新入口；该文件可能包含迁移时解析出的环境值。

## 更新代码到服务器：用 rsync，不用 git pull

代码先提交并推送到 GitHub，再用**本机 rsync 直传**到服务器。沿用此流程可避免生产更新依赖服务器访问 GitHub；旧服务器的网络故障不代表新服务器也有同样限制。

标准流程：

```bash
# 1. 本机改代码、commit、push 到 GitHub（正常走 git 工作流，仓库是唯一代码源头）
git push origin main

# 2. rsync 同步到服务器，排除凭据、备份和迁移记录
rsync -avz -e 'ssh -p 7339' --exclude='.git' --exclude='.env' --exclude='.codegraph' --exclude='node_modules' --exclude='backups' --exclude='compose.migrated.json' \
  /Users/wuhaoyuan/personal_code/vocabulary-memorization/ \
  root@123.56.219.4:/root/vocabulary-memorization/

# 3. 服务器上重新构建并启动
ssh -p 7339 root@123.56.219.4 "cd /root/vocabulary-memorization && docker compose -f docker-compose.yml -f docker-compose.production.yml up -d --build"
```

首次部署才需要执行 `./deploy.sh`（会自动生成 `.env` 里的随机数据库密码和超管密码），之后更新代码只需要上面第 2、3 步，`.env` 不会被覆盖。同步不加 `--delete`，保留服务器上的迁移记录和其他运维文件；迁移到新机器时还需单独迁移 `.env`、MySQL 与音频数据卷。

## 涉及数据库结构变更时：先备份，再部署，再验证迁移结果

后端启动时的 `storage.Migrate()`（`backend/internal/storage/migrations.go`）会自动、幂等地跑完所有建表 / 加列 / 历史数据回填，**不需要手工连数据库执行 SQL**。但只要这次更新涉及表结构变更（加表、加列、扫表回填一类），部署前建议按下面的顺序操作，而不是直接第 3 步覆盖重建：

```bash
# 1. 全库备份保存在服务器目录，重建 mysql 容器后仍可读取；密码只在容器内展开
ssh -p 7339 root@123.56.219.4 "cd /root/vocabulary-memorization && umask 077 && mkdir -p backups && set -C && docker compose -p vocabulary-memorization exec -T mysql sh -c 'MYSQL_PWD=\"\$MYSQL_ROOT_PASSWORD\" mysqldump -uroot --all-databases' > backups/backup_pre_<版本号>.sql && test -s backups/backup_pre_<版本号>.sql"

# 2. rsync 同步代码、加载基础与生产覆盖文件后 up -d --build（同上）

# 3. 部署后检查迁移是否跑成功：新表/新列是否存在，数据量是否符合预期
ssh -p 7339 root@123.56.219.4 "cd /root/vocabulary-memorization && docker compose -f docker-compose.yml -f docker-compose.production.yml logs backend --tail=60"
ssh -p 7339 root@123.56.219.4 "cd /root/vocabulary-memorization && docker compose -f docker-compose.yml -f docker-compose.production.yml exec -T mysql sh -c 'mysql -uroot -p\"\$MYSQL_ROOT_PASSWORD\" vocab -e \"SHOW TABLES; DESCRIBE <新表或改动的表>;\"'"

# 4. 冒烟测试：主要页面和一个不需要登录也能验证的接口
curl -s -o /dev/null -w "%{http_code}\n" http://123.56.219.4:39100/
curl -s http://123.56.219.4:39100/api/me   # 未登录应返回 401，不应该 500
```

备份使用服务器现有默认 Compose 文件并固定项目名，首次源码更新前也能执行，不依赖尚未同步的生产覆盖文件。首次更新前查看现有状态或日志，同样可以使用 `docker compose -p vocabulary-memorization ps` / `logs`。

备份位于服务器 `/root/vocabulary-memorization/backups/`，不依赖旧容器的临时目录；备份目录与迁移 JSON 已被 Git 和镜像构建上下文排除。部署前确认备份命令退出成功、文件非空；失败则停止部署。不要重复使用同一版本的备份文件名覆盖已有备份。

需要整体恢复时，在服务器确认恢复目标与备份版本，再执行：

```bash
cd /root/vocabulary-memorization
docker compose -f docker-compose.yml -f docker-compose.production.yml exec -T mysql \
  sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot' < backups/backup_pre_<版本号>.sql
```

备份文件建议部署验证通过、稳定运行几天后再手动清理，不要部署完立刻删。

## Go 模块代理：goproxy.cn / npm 镜像：npmmirror

`backend/Dockerfile` 里加了两行，分别对应 Go 后端构建阶段和前端构建阶段：

```
ENV GOPROXY=https://goproxy.cn,direct
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
```

当前保留这两个构建镜像源。它们是旧部署环境中为解决下载超时加入的配置；新服务器尚未执行本次源码构建，不能据此断言默认源一定超时。

如果部署时发现 `npm ci` 仍然卡死/超时（镜像也不稳定的极端情况），退回备选方案：本机先 `cd frontend && npm run build`，再把 `frontend/dist/` 一起 rsync 到服务器，把 `backend/Dockerfile` 里的 `frontend-builder` stage 去掉，改成直接 `COPY frontend/dist ./static`。

## 端口：39100，不是默认的 8080

`docker-compose.yml` 里 backend 的端口映射是 `"39100:8080"`（容器内仍然监听 8080）。

新服务器继续使用 39100，SSH 使用 7339。2026-10-08 已确认两个端口可达，但未查询云安全组的完整规则，不沿用旧服务器的端口范围、占用或 ufw 结论。

如果以后要换应用端口，改基础 Compose 的端口映射，确认云安全组与服务器防火墙放行新端口，再加载基础与生产覆盖文件重新创建容器。

## DeepSeek API Key 配置：只能生效一次种子值

`.env` 里的 `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` 只有在数据库 `settings` 表里**完全没有这条记录**时（即容器第一次启动）才会被拿去初始化数据库。

这意味着：**部署过一次之后，再改 `.env` 里的这几个值、重启容器，是不会生效的**——数据库里已经有记录了，代码只在"缺失"时才种入。

正确的改法：登录后台管理页面 `http://123.56.219.4:39100/admin`（超管账号），在设置里改，改完立即生效，不需要重启容器。

## 安全：不要把密钥写进代码

之前 `deploy.sh` 和 `backend/internal/app/settings.go` 里各硬编码过一份真实的 DeepSeek API Key 作为默认值，被 GitHub push protection 拦截，后来把这两处默认值改成了空字符串，并清理了本地未推送提交的 git 历史。以后 DeepSeek Key 只应该存在于服务器上的 `.env`（不进 git，已在 `.gitignore` 里）或数据库（后台管理页面改）。

## 常用命令

```bash
ssh -p 7339 root@123.56.219.4

cd /root/vocabulary-memorization
docker compose -f docker-compose.yml -f docker-compose.production.yml ps               # 查看容器状态
docker compose -f docker-compose.yml -f docker-compose.production.yml logs -f backend  # 后端日志
docker compose -f docker-compose.yml -f docker-compose.production.yml logs -f mysql    # 数据库日志
docker compose -f docker-compose.yml -f docker-compose.production.yml up -d --build    # 更新代码后构建启动
docker compose -f docker-compose.yml -f docker-compose.production.yml down             # 停止服务（数据卷保留）
```

## 超管账号

用户名/密码在服务器的 `/root/vocabulary-memorization/.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD`，首次部署时随机生成，不会被后续更新覆盖。

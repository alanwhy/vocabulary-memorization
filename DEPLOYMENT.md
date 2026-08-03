# 部署说明（当前实际部署逻辑）

记录的是这台服务器上**实际跑起来的**部署方式，和 README 里的通用说明相比多了几个针对这台服务器踩坑后的调整。

## 服务器信息

- 地址：`101.42.45.60`（root 用户，本机已配置 SSH 密钥免密登录）
- 部署目录：`/root/vocabulary-memorization`
- 访问地址：`http://101.42.45.60:39100`
- 技术栈：Docker Compose 起两个容器 —— `mysql:8.0` + Go 后端（`backend/Dockerfile` 构建）

## 更新代码到服务器：用 rsync，不用 git pull

服务器访问 GitHub 的网络不稳定（`git clone`/`git pull` 经常 TLS 连接被重置或超时），所以代码同步走的是 **本机 rsync 直传**，而不是在服务器上拉仓库。

标准流程：

```bash
# 1. 本机改代码、commit、push 到 GitHub（正常走 git 工作流，仓库是唯一代码源头）
git push origin main

# 2. rsync 同步到服务器（排除 .git / .env / .codegraph，避免覆盖服务器上的密钥配置）
rsync -avz --exclude='.git' --exclude='.env' --exclude='.codegraph' --exclude='node_modules' \
  /Users/wuhaoyuan/personal_code/vocabulary-memorization/ \
  root@101.42.45.60:/root/vocabulary-memorization/

# 3. 服务器上重新构建并启动
ssh root@101.42.45.60 "cd /root/vocabulary-memorization && docker compose up -d --build"
```

首次部署才需要执行 `./deploy.sh`（会自动生成 `.env` 里的随机数据库密码和超管密码），之后更新代码只需要上面第 2、3 步，`.env` 不会被覆盖。

## Go 模块代理：goproxy.cn

`backend/Dockerfile` 里加了一行：

```
ENV GOPROXY=https://goproxy.cn,direct
```

没有这行的话，`go mod tidy` 会去请求 `proxy.golang.org`，在这台服务器的网络环境下会直接超时，构建失败。

## 端口：39100，不是默认的 8080

`docker-compose.yml` 里 backend 的端口映射是 `"39100:8080"`（容器内仍然监听 8080）。

原因：这台服务器的云安全组只放行了 `39000-40000` 这个区间的端口（历史遗留，`39001`/`39002`/`39010` 已经被 nginx 和宝塔面板占用），没有单独放行 8080。为了不用去云控制台改安全组，直接把宿主机映射端口换成这个区间里空闲的 `39100`，安全组和 ufw 都不需要额外配置。

> 遗留项：服务器 ufw 里还留着一条历史调试时加的 `8080/tcp ALLOW` 规则，现在没用了，可以找机会 `ufw delete allow 8080/tcp` 清掉。

如果以后要换端口，改 `docker-compose.yml` 里的端口号，确认新端口在 `39000-40000` 区间内（或者去安全组新开一个端口），然后 `docker compose up -d` 重新创建容器即可。

## DeepSeek API Key 配置：只能生效一次种子值

`.env` 里的 `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` 只有在数据库 `settings` 表里**完全没有这条记录**时（即容器第一次启动）才会被拿去初始化数据库。

这意味着：**部署过一次之后，再改 `.env` 里的这几个值、重启容器，是不会生效的**——数据库里已经有记录了，代码只在"缺失"时才种入。

正确的改法：登录后台管理页面 `http://101.42.45.60:39100/admin.html`（超管账号），在设置里改，改完立即生效，不需要重启容器。

## 安全：不要把密钥写进代码

之前 `deploy.sh` 和 `backend/settings.go` 里各硬编码过一份真实的 DeepSeek API Key 作为默认值，被 GitHub push protection 拦截，后来把这两处默认值改成了空字符串，并清理了本地未推送提交的 git 历史。以后 DeepSeek Key 只应该存在于服务器上的 `.env`（不进 git，已在 `.gitignore` 里）或数据库（后台管理页面改）。

## 常用命令

```bash
ssh root@101.42.45.60

cd /root/vocabulary-memorization
docker compose ps                  # 查看容器状态
docker compose logs -f backend     # 后端日志
docker compose logs -f mysql       # 数据库日志
docker compose up -d --build       # 更新代码后重新构建启动
docker compose down                # 停止服务（数据库数据保留）
```

## 超管账号

用户名/密码在服务器的 `/root/vocabulary-memorization/.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD`，首次部署时随机生成，不会被后续更新覆盖。

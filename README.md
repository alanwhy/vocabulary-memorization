# 背单词

极简的背单词网页：登录后输入英文单词回车即记录，自动查词得到中文释义（含词性，一个词有多个词性会分别列出），重复输入同一个词会累计背诵次数。列表按背诵次数降序排列，次数相同时按最近一次背诵时间降序排列。每个账号的背单词记录互相隔离，手机和电脑用同一个账号登录看到的是同一份数据。深色/浅色主题跟随系统或浏览器设置自动切换。

## 用户体系

- 应用整体需要登录才能使用，没有自助注册入口。
- 首次启动会自动创建一个超管账号（用户名/密码见 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD`，`./deploy.sh` 首次部署时会随机生成）。
- 超管登录后可以在右上角「后台管理」（`/admin.html`）手动新增用户（可指定是否也是超管）。
- 普通用户只能看到、操作自己的单词列表。

## DeepSeek 查词

- 查词优先级：DeepSeek（如果已启用并配置）→ Google 免费翻译接口兜底。
- DeepSeek 返回的释义按词性拆开结构化存储，一个词有几个常见词性就展示几条。
- API Key / Base URL / 模型名 / 是否启用这四项配置存在数据库里，首次启动会用环境变量 `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` 做初始值种进数据库，之后可以在「后台管理」页面随时修改，改完立即生效，不需要重启服务。

## 目录结构

```
vocabulary-memorization/
├── backend/                Go 后端源码
│   ├── main.go              路由 + 录入/列表/删除接口 + 用户管理接口
│   ├── db.go                MySQL 连接 + 幂等数据库迁移
│   ├── auth.go               登录/登出/会话中间件、超管账号引导
│   ├── settings.go            DeepSeek 配置的读写与内存缓存
│   ├── deepseek.go            调用 DeepSeek 查词
│   ├── translate.go          翻译逻辑：DeepSeek -> 在线接口兜底
│   ├── models.go
│   ├── static/index.html       背单词页面（Vue 3，CDN 引入，无需构建）
│   ├── static/admin.html       后台管理页面（用户管理 + DeepSeek 配置）
│   ├── schema.sql              数据库表结构
│   └── Dockerfile
├── docker-compose.yml       编排 MySQL + 后端两个容器
├── deploy.sh                 一键部署/更新脚本
└── README.md
```

## 技术说明

- 后端：Go（标准库 net/http 路由 + go-sql-driver/mysql + golang.org/x/crypto/bcrypt），无第三方 web 框架。
- 数据库：MySQL 8，数据存在 Docker volume 里，容器重启不丢数据。老版本（没有用户体系）升级上来也不会丢历史单词，启动时会自动把老数据挂到超管账号名下。
- 认证：登录后用 HttpOnly Cookie 保存会话 token，会话有效期 30 天并会随访问自动续期。
- 翻译：先查 DeepSeek（未配置或调用失败会跳过）→ 再调用 Google 免费翻译接口兜底。两个来源都查不到时单词依然会正常入库，只是释义留空。
- 前端：`static/index.html`（背单词）+ `static/admin.html`（后台管理），都是 Vue 3 CDN 单文件页面，没有打包步骤。

## 部署到服务器（101.42.45.60）

1. 把整个 `vocabulary-memorization` 文件夹上传到服务器，比如 `/root/vocabulary-memorization`（可以用 `scp -r` 或者 `rsync`）。
2. SSH 登录服务器（root），进入目录：
   ```bash
   cd /root/vocabulary-memorization
   chmod +x deploy.sh
   ./deploy.sh
   ```
3. 脚本会自动生成随机数据库密码、超管密码（存在 `.env` 里，同时预填了 DeepSeek 的 Key/Base URL/模型，之后可在后台管理页面改），然后执行 `docker compose up -d --build`，构建后端镜像并启动 MySQL + 后端两个容器。
4. 部署完成后，浏览器访问 `http://101.42.45.60:8080`，用 `.env` 里的 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 登录（超管账号），手机和电脑用同一个账号登录即可看到同一份数据。

`.env` 现在已经加入 `.gitignore`，不会再被提交到仓库里（里面存的是数据库密码、超管密码和 DeepSeek API Key），换新机器部署时记得把这个文件也一起拷过去，不然会重新生成一套新的密码/账号。

### 端口没打开怎么办

`docker compose up` 成功不代表外网能访问到，8080 端口需要在云服务商控制台的**安全组**里放行（这是最常见的坑），如果服务器本机还开了 `ufw` 之类的防火墙，也要 `ufw allow 8080`。

### 想换端口

编辑 `docker-compose.yml` 里 backend 服务的 `ports`，比如改成 `"9000:8080"`，然后重新执行 `./deploy.sh`。

### 更新代码后如何重新部署

改完代码，直接再执行一次 `./deploy.sh` 就会重新构建镜像并重启容器，`.env` 里的密码不会变、数据库数据也不会丢。

### 常用命令

```bash
docker compose logs -f backend   # 看后端日志（比如翻译接口报错、启动报错都在这里）
docker compose logs -f mysql     # 看数据库日志
docker compose down              # 停止服务
docker compose down -v           # 停止并清空数据库数据（慎用）
```

## 重要提示：这份代码尚未实际编译运行过

我（Claude）目前所在的沙盒环境里装的 Go 版本太老（1.13），编不过这个项目要求的 Go 1.24，也连不上外网下载依赖，所以用户体系、DeepSeek 查词这两块新代码同样是纯手写完成的，**没有在本地编译或运行验证过**。代码逻辑上我逐文件仔细检查过，但涉及数据库迁移、会话认证这类逻辑，建议部署后按这个顺序手动过一遍：

1. 用超管账号登录，确认能进背单词页面。
2. 加几个单词，包括故意加一个多词性的词（比如 `book`），检查释义是否按词性分开展示、翻译计数是否正常。
3. 去「后台管理」新增一个普通用户，退出后用这个新账号登录，确认列表是空的、和超管的列表互不影响。
4. 在「后台管理」里改一下 DeepSeek 配置（比如换个不存在的 key 再换回来），加一个新词，确认查词走的是最新配置。

如果执行 `./deploy.sh` 时遇到编译报错或者启动报错，把报错信息发给我，我可以照着修。

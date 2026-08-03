# 背单词

极简的背单词网页：输入英文单词回车即记录，自动翻译成中文释义（含词性），重复输入同一个词会累计背诵次数。列表按背诵次数降序排列，次数相同时按最近一次背诵时间降序排列。手机和电脑访问的是同一个服务、同一份数据。

## 目录结构

```
vocabulary-memorization/
├── backend/                Go 后端源码
│   ├── main.go              路由 + 录入/列表/删除接口
│   ├── db.go                MySQL 连接
│   ├── dict.go               内置词典加载（go:embed）
│   ├── translate.go          翻译逻辑：内置词典优先，查不到再调用在线接口兜底
│   ├── models.go
│   ├── dict/words.json        内置词典数据（约 250 个常见词，含词性）
│   ├── static/index.html       前端页面（Vue 3，CDN 引入，无需构建）
│   ├── schema.sql              数据库表结构
│   └── Dockerfile
├── docker-compose.yml       编排 MySQL + 后端两个容器
├── deploy.sh                 一键部署/更新脚本
└── README.md
```

## 技术说明

- 后端：Go（标准库 net/http 路由 + go-sql-driver/mysql），无第三方 web 框架。
- 数据库：MySQL 8，数据存在 Docker volume 里，容器重启不丢数据。
- 翻译：先查内置词典（离线、免费、约 250 个常见词），查不到则调用有道网页翻译的免注册免费接口兜底（没有 API Key，稳定性一般，属于非官方接口）。如果两者都没查到，单词依然会正常入库，只是释义留空，你可以之后自己去数据库补，或者后续换成正式的百度翻译开放平台 / 有道智云 API（在 `backend/translate.go` 里替换 `translateOnline` 函数即可，我已经把翻译逻辑单独抽出来，方便替换）。
- 前端：单文件 `static/index.html`，Vue 3 通过 CDN（unpkg）引入，没有打包步骤。

## 部署到服务器（101.42.45.60）

1. 把整个 `vocabulary-memorization` 文件夹上传到服务器，比如 `/root/vocabulary-memorization`（可以用 `scp -r` 或者 `rsync`）。
2. SSH 登录服务器（root），进入目录：
   ```bash
   cd /root/vocabulary-memorization
   chmod +x deploy.sh
   ./deploy.sh
   ```
3. 脚本会自动生成随机数据库密码（存在 `.env` 里），然后执行 `docker compose up -d --build`，构建后端镜像并启动 MySQL + 后端两个容器。
4. 部署完成后，浏览器访问 `http://101.42.45.60:8080`，手机和电脑用同一个地址即可看到同一份数据。

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

我（Claude）目前所在的沙盒环境里没有装 Go 和 Docker，也连不上外网下载依赖，所以这套代码是纯手写完成的，**没有在本地编译或运行验证过**。代码逻辑上我逐文件仔细检查过，结构也比较简单，正常情况下应该可以直接跑起来，但如果你在服务器上执行 `./deploy.sh` 时遇到编译报错或者启动报错，把报错信息发给我，我可以照着修。建议第一次部署完先自己测试一下：输入几个单词，看看翻译和计数是否正常，再放心用起来。

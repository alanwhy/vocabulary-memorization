# 开发者贡献指南

这份文档面向要改这个项目代码的人：怎么在本地把服务跑起来、怎么跑测试、写代码时有哪些必须遵守的约定、改完怎么发版。

- 项目是什么、有哪些功能 → [README.md](README.md)
- 部署到生产服务器的实操流程 → [DEPLOYMENT.md](DEPLOYMENT.md)
- 每个版本改了什么 → [CHANGELOG.md](CHANGELOG.md)

## 环境要求

| 依赖 | 版本 | 说明 |
|---|---|---|
| Go | 1.24+ | 见 `backend/go.mod` |
| Node.js | `^22.18.0 \|\| >=24.12.0` | 见 `frontend/package.json` 的 `engines` |
| MySQL | 8.x | 本地一般用 Docker 起，不用装到系统里 |
| Docker + Docker Compose | 任意近期版本 | 跑 MySQL / 验收整套服务 |

## 本地启动

有两种方式，**日常开发用方式 B**（前端有热更新，改一行 CSS 立刻能看到），方式 A 只适合最后验收「和线上一模一样的产物」。

### 方式 A：一条命令跑整套（Docker Compose）

```bash
# 仓库根目录下需要有 .env（首次可以直接跑 ./deploy.sh 生成，或手工照下面的变量表写一份）
docker compose up -d --build
```

访问 http://localhost:39100 ，用 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 登录。

```bash
docker compose logs -f backend   # 看后端日志
docker compose down              # 停止（数据库数据保留在 volume 里）
docker compose down -v           # 停止并清空数据库（慎用）
```

注意：前端产物是**构建进镜像**的，所以改任何一行前端代码都要 `docker compose up -d --build` 重建才能看到效果，一次几十秒。别用这个方式调样式。

### 方式 B：开发模式（前端热更新）

分三步：MySQL 用容器、后端 `go run`、前端 `npm run dev`。

**1. 起一个能从宿主机连的 MySQL**

`docker-compose.yml` 里的 mysql 只在 compose 内网暴露 3306，宿主机连不上，所以开发时单独起一个：

```bash
# 在仓库根目录执行（要挂 schema.sql，路径依赖当前目录）
docker run -d --name vocab-mysql-dev -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=devroot \
  -e MYSQL_DATABASE=vocab \
  -e MYSQL_USER=vocab \
  -e MYSQL_PASSWORD=devpass \
  -v "$PWD/backend/schema.sql:/docker-entrypoint-initdb.d/schema.sql:ro" \
  mysql:8.0
```

`docker-entrypoint-initdb.d` 只在**数据目录为空时**执行，也就是只有这个容器第一次启动会导入 `schema.sql`。

> ⚠️ **一定要导入 `schema.sql`**：`words` 表只在 `schema.sql` 里定义，Go 的 `migrateSchema()` 不会建它（它只负责补 `users`/`sessions`/`settings`/`word_dictionary` 和后续的加列、加索引、历史数据回填）。对着一个完全空的库直接跑后端，会在第一次操作单词时报表不存在。

如果你更愿意用系统里已装的 MySQL：`mysql -uroot -p < backend/schema.sql` 手工导入一次即可，然后把下一步的连接参数改成你自己的。

**2. 起后端**

```bash
cd backend
DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=vocab DB_PASSWORD=devpass DB_NAME=vocab \
ADMIN_USERNAME=admin ADMIN_PASSWORD=admin123456 \
go run .
```

后端固定监听 `:8080`（写死在 `main.go`，没有 `PORT` 环境变量）。首次启动会：连库 → 跑幂等迁移 → 没有超管账号时用 `ADMIN_USERNAME`/`ADMIN_PASSWORD` 建一个（不设 `ADMIN_PASSWORD` 就随机生成一个并打在日志里）。

**3. 起前端**

```bash
cd frontend
npm install      # 首次
npm run dev
```

访问 Vite 打印的地址（默认 http://localhost:5173 ）。`vite.config.js` 里把 `/api` 代理到 `http://localhost:8080`，所以前后端两个端口不需要处理 CORS。

**关于查词**：没配 `DEEPSEEK_API_KEY` 时会走 Google 免费翻译接口兜底；国内网络下这个接口大概率连不通，表现是新词在「查词中…」转几轮重试后落到「暂无释义」——这是预期行为，不是 bug。想在本地看到真实释义，就在启动后端时带上 `DEEPSEEK_API_KEY`，或者输入一个词库里已经缓存过的词（会直接命中缓存，秒回）。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `mysql` / `3306` / `vocab` / 空 / `vocab` | 默认值是给 compose 内网用的，本地开发要显式指定 `DB_HOST=127.0.0.1` |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | 25 / 25 | 连接池上限 |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / 随机 | 只在**库里一个超管都没有**时生效，之后改这两个值不会改已有账号的密码 |
| `COOKIE_SECURE` | `false` | 走 HTTPS 的环境才需要设 `true`，本地 http 下设了会导致 Cookie 存不下、登录失败 |
| `MAX_CONCURRENT_TRANSLATIONS` | 5 | 后台查词的并发上限（信号量） |
| `DEEPSEEK_API_KEY` / `DEEPSEEK_BASE_URL` / `DEEPSEEK_MODEL` | 空 / `https://api.deepseek.com` / `deepseek-v4-flash` | **只在 `settings` 表里没有这条记录时**（即数据库第一次初始化）被种进数据库，之后改环境变量无效，要在后台管理页面改 |

`.env` 在 `.gitignore` 里，不要提交。它只被 `docker compose` 读取，`go run` 不会自动加载。

## 常用命令

```bash
# 后端
cd backend
go vet ./...            # 静态检查
go test ./...           # 全部单元测试（46 个，不连数据库，秒级）
go test -run TestParsePagination ./...

# 前端
cd frontend
npm run lint            # oxlint + eslint，带 --fix
npm run build           # 产物进 dist/（Docker 构建时会拷到 backend/static/）
npm run preview         # 本地预览构建产物
```

提交前至少跑一遍 `go vet ./... && go test ./...` 和 `npm run lint && npm run build`。

## 代码约定

### 注释写「为什么」，不写「是什么」

这个仓库的注释密度不低，但都是在解释**为什么这么写**——踩过的坑、被否掉的方案、不能改的约束。比如：

```go
// wordOrderBy 把前端传来的排序模式映射成固定的 ORDER BY 片段。
// 这里必须走白名单：SQL 片段是拼接进语句的，绝不能让请求参数直接落进去。
// 每种排序都以 id 收尾——review_count / last_reviewed_at / word_key 都不唯一，
// 缺少唯一 tiebreaker 时 LIMIT/OFFSET 翻页会出现跨页重复或漏行。
```

不要写 `// 遍历列表` 这种复述代码的注释。中文注释，和现有风格保持一致。

### 后端分层：handler → 窄接口 → repository

```
main.go / auth.go / dictionary.go / settings.go   handler（解析请求、鉴权、写响应）
app.go                                            App 结构体 + userStore/wordStore/... 窄接口
store.go                                          所有 SQL 都在这里，一个 repo 一个 struct
```

- **SQL 只写在 `store.go`**，handler 里不出现 `db.Query`。
- handler 是 `*App` 的方法，依赖通过 `app.go` 里的窄接口拿，这样测试能塞 fake 进去。
- 新增一个接口的完整步骤：
  1. `store.go` 给对应的 repo 加方法
  2. `app.go` 在窄接口里加同名方法签名
  3. 写 handler 方法
  4. `main.go` 的 `mux` 注册路由，统一套 `withTimeout(defaultRequestTimeout)(app.requireAuth(...))`（管理员接口用 `requireAdmin`）
  5. `fakes_test.go` 里对应的 fake 补上这个方法（不补的话整个测试包编译不过）
  6. 加测试

### 数据库迁移必须幂等

表结构变更写在两个地方：`schema.sql`（新装机器用）+ `db.go` 的迁移函数（老部署用），**不要让任何人手工连数据库执行 SQL**。迁移函数必须用 `columnExists` / `indexExists` 守卫，保证反复启动都是空操作：

```go
func migrateUsersLastLoginColumn() {
	if columnExists("users", "last_login_at") {
		return
	}
	mustExec(`ALTER TABLE users ADD COLUMN last_login_at DATETIME NULL`)
}
```

写完记得在 `migrateSchema()` 里按顺序调用。

### 分页三条铁律

1. **响应用统一信封**：`newPageResult(items, total, page, limit)` → `{items, total, page, limit, has_more}`，`has_more` 由后端算，前端不要自己拿 `total` 和 `page` 推。
2. **`ORDER BY` 走白名单**：排序片段是字符串拼接进 SQL 的，请求参数只能用来在 `switch` 里选分支（见 `wordOrderBy`），绝不能拼进语句。
3. **每种排序都要有唯一 tiebreaker**：结尾必须带 `id`。按不唯一的列做 `LIMIT/OFFSET` 翻页，会出现同一条记录在两页都出现、另一条谁都不出现。`store_test.go` 里有个测试专门守这条不变量。

另外，`LIKE` 的关键字一律走 `likeContains()`（内部用 `escapeLikePattern()` 转义 `\ % _` 再包 `%…%`），不要自己拼 `"%"+keyword+"%"`，否则用户输入的 `%` 会变成通配符。

### 测试：fake 优先，纯函数优先

- 单元测试**不连数据库**。仓储层通过 `fakes_test.go` 里手写的 fake 替换（不用 mock 框架），handler 测试用 `httptest` 打 `App` 的方法。
- 排序映射、分页参数夹取、LIKE 转义、id 列表解析这类逻辑都刻意抽成了纯函数，方便直接测边界值——新写逻辑时优先照这个思路拆。
- 涉及安全的地方（SQL 拼接、鉴权、限流）必须有测试，包括恶意输入的用例，比如 `wordOrderBy("1; DROP TABLE words")` 要落到默认分支。

### 前端约定

- 重复三次以上的逻辑抽成 composable（`src/composables/`）或纯函数（`src/utils/`）。已有的：`usePaginatedList`（分页状态机）、`useInfiniteScroll`（滚动加载）、`useTheme`（主题开关）、`useWordActions`（归档/删除）、`format.js`（时间格式化）、`reviewLevel.js`（次数档位）。别再在组件里抄第二份 `formatTime`。
- 列表数据一律走 `usePaginatedList`，它内部用自增的 `reqSeq` 丢弃过期响应：切换排序/过滤时，慢的旧请求返回晚了会覆盖新结果，必须按序号丢。
- 主题：所有颜色走 CSS 变量，浅色定义在 `:root`、深色在 `html.dark`（`assets/main.css`）。**不要新增 `@media (prefers-color-scheme: dark)`** ——主题是用户手动选的，不跟随系统。Element Plus 的深色变量也挂在 `html.dark` 上，正好共用同一个开关。
- 样式默认写 `scoped`；只有需要跨组件复用的（如 `.count-badge`）才放进 `main.css` 做全局 class。改配色时注意浅色和深色两套变量都要给。
- 请求统一走 `api/client.js`（`apiGet`/`apiPost`/`apiPut`/`apiDelete`），它统一处理凭证、401 跳转和错误归一化，不要在组件里裸写 `fetch`。
- 路径别名 `@/` 指向 `frontend/src`。

## 提交与发版

1. 分支：直接在 `main` 上开发也接受（个人项目），但涉及多文件改动建议开分支再合。
2. Commit message：首行 `类型: 中文摘要`（`feat:` / `fix:` / `docs:` / `refactor:`），改动多的话空一行后列要点。参考 `git log`。
3. 发版时三处一起改：
   - `CHANGELOG.md` 加一个版本条目（`Added` / `Fixed` / `Changed` / `Security` 分段，中文）
   - `frontend/src/views/HomeView.vue` 底部的 footer 版本号
   - `frontend/package.json` 的 `version`；改完跑一次 `npm install --package-lock-only` 同步 lockfile（Docker 构建用 `npm ci`，两者版本不一致会构建失败）
4. 部署：`git push origin main` 之后按 [DEPLOYMENT.md](DEPLOYMENT.md) 走（rsync 到服务器 + `docker compose up -d --build`，服务器网络连 GitHub 不稳定，不要在服务器上 `git pull`）。**这次改动涉及表结构变更时，先在服务器上 `mysqldump` 备份，再部署，再验证新列/新索引是否补上。**

## 常见坑

| 现象 | 原因 |
|---|---|
| 本地登录成功但立刻被踢回登录页 | 设了 `COOKIE_SECURE=true` 但访问的是 http，浏览器不存 Cookie |
| 报表不存在 / 单词操作 500 | 空库没导入 `backend/schema.sql`，`words` 表不存在（Go 迁移不建这张表） |
| 改了 `.env` 里的 DeepSeek Key 但不生效 | 这几个值只在 `settings` 表没记录时种入，之后要在后台管理页面改 |
| 改了 `.env` 里的 `ADMIN_PASSWORD` 但登录不上 | 只在库里没有任何超管时才建账号；已有账号请用后台的重置密码功能 |
| 前端改了代码但页面没变 | 用的是方式 A（Docker），产物构建进镜像了，要 `--build` 重建；或者浏览器缓存，`Cmd+Shift+R` |
| `npm ci` 在服务器上超时 | 见 DEPLOYMENT.md，`Dockerfile` 里已配 npmmirror / goproxy.cn 镜像 |
| 新词一直显示「暂无释义」 | 没配 DeepSeek Key，Google 兜底接口在国内连不通，重试耗尽后留空 |

<!-- 阶段记录：模型 GPT-5 (Codex) / 开始 2026-09-18 15:45 CST / 结束 2026-09-18 15:47 CST / 规范版本 @sw/dev-skills@1.1.2 -->

# 实现计划

## 背景

- **任务类型**：refactor
- **一句话做什么**：在保持所有运行行为不变的前提下，将 Go 后端从单一 `package main` 拆成入口、应用、存储和领域模型四个明确依赖方向的包。

## 改动方案

### 改动清单

- [x] `backend/**/*.go` —— 建立改动前测试与编译基线 —— `cd backend && go test ./...` 输出 `ok vocab-backend`，`go build ./...` 无输出且退出码为 0。
- [x] `backend/internal/model/models.go`、`backend/internal/model/models_test.go`、`backend/internal/model/normalize_test.go` —— 迁移共享模型、SRS/释义合并和重要释义规范化规则，导出跨包所需符号但保持 JSON 标签与算法不变 —— 单独执行 `go test ./internal/model`。（依赖第 1 项）
- [x] `backend/internal/storage/database.go`、`backend/internal/storage/migrations.go` —— 将全局数据库连接和迁移改为接收显式 `*sql.DB`，保持连接参数、重试、连接池和迁移顺序不变 —— `go test ./internal/storage` 可编译，核对迁移调用顺序。（依赖第 2 项）
- [x] `backend/internal/storage/users.go`、`sessions.go`、`words.go`、`dictionary.go`、`settings.go` —— 将 `store.go` 按聚合拆文件，统一返回 `internal/model` 类型并提供 repository 构造入口 —— `go test ./internal/storage`。（依赖第 2、3 项）
- [x] `backend/internal/storage/store_test.go` —— 迁移原 `store_test.go` 中属于 storage 的排序白名单、LIKE 转义、词义筛选和 NULL 时间白盒测试；分页响应测试留给 app —— `go test ./internal/storage`。（依赖第 4 项）
- [x] `backend/internal/app/app.go`、`env.go`、`lifecycle.go`、`model_aliases.go`、`routes.go`、`response.go`、`middleware.go`、`auth.go`、`words.go`、`dictionary.go`、`settings.go`、`pronunciation.go`、`translate.go`、`deepseek.go`、`doubao.go`、`ratelimit.go` —— 迁移 HTTP 与业务编排；`App` 通过 `Dependencies` 接收 store 接口，集中注册原路由并暴露进程生命周期所需的最小方法 —— `go test ./internal/app` 可编译，依赖图中不存在 `app → storage`。（依赖第 2、4 项）
- [x] `backend/internal/app/auth_test.go`、`deepseek_test.go`、`dictionary_test.go`、`doubao_test.go`、`fakes_test.go`、`main_test.go`、`response_test.go`、`routes_test.go`、`middleware_test.go`、`ratelimit_test.go`、`settings_test.go` —— 将白盒测试随实现迁移，把原 `store_test.go` 的分页响应测试归入 app；补充路由契约测试覆盖“请求 → 路由 → 中间件 → Handler → fake store → HTTP 响应” —— `go test ./internal/app`。（依赖第 6 项）
- [x] `backend/cmd/vocab-server/main.go` —— 建立唯一组合根，按原顺序完成数据库初始化、依赖注入、后台任务启动、HTTP 服务和优雅关闭 —— `go build ./cmd/vocab-server`。（依赖第 3、4、6 项）
- [x] `backend/Dockerfile`、`README.md`、`CONTRIBUTING.md`、`DEPLOYMENT.md` —— 将构建/运行命令和源码路径改为 `./cmd/vocab-server` 及新目录，不改变二进制名、端口、工作目录或 Compose 行为 —— 检索旧入口命令和旧源码路径，确认均已更新。（依赖第 8 项）
- [x] `backend/` 根目录原 `.go` 文件 —— 确认实现与测试均已有新归属后删除旧平铺副本，避免重复实现 —— `find backend -maxdepth 1 -name '*.go'` 无结果。（依赖第 2 至 8 项）
- [x] `backend/**/*.go` —— 对迁移后的 Go 文件统一格式化并检查包依赖 —— `gofmt` 后 `go list ./...` 输出 `vocab-backend/cmd/vocab-server`、`internal/app`、`internal/model`、`internal/storage`，且无 import cycle。（依赖第 10 项）
- [x] `backend/**/*_test.go` —— 运行完整回归并确认测试没有减少 —— `cd backend && go test ./...` 全部通过。（依赖第 11 项）
- [x] `backend/**/*.go` —— 执行最终可编译性验证 —— `cd backend && go build ./...` 无输出且退出码为 0。（依赖第 12 项）

### 本次不做

- 不引入 Gin、Echo、Fiber、Chi、GORM、sqlc 或依赖注入框架。
- 不修改 HTTP method/path、请求响应字段、状态码、鉴权或分页语义。
- 不修改数据库 schema、SQL 业务语义或历史迁移逻辑。
- 不重写翻译、TTS、SRS、限流和后台任务算法。
- 不启动开发服务、不操作浏览器、不部署生产环境。
- 不为了形式创建空转发 `service` 层，也不把每个 repository 拆成独立 package。

## 关键文件

CodeGraph 摘要：

```text
Callers of "NewApp" (1):
function main — backend/main.go:54

Callees of "NewApp" (9):
getEnvInt, newAttemptTracker, App,
userRepo, sessionRepo, wordRepo, dictionaryRepo, settingsRepo, Config
```

| 文件 | 改动性质 | 具体改动 | callers/callees 摘要 |
|---|---|---|---|
| `backend/main.go` → `backend/cmd/vocab-server/main.go` | 移动并收窄 | 只保留组合、启动与关闭 | 当前唯一调用 `NewApp`，并调用 DB、迁移、后台任务和 HTTP 服务 |
| `backend/app.go` → `backend/internal/app/app.go` | 移动并修改 | `App`、store 接口、`Dependencies` 注入 | 当前反向创建五种具体 repository；测试 fake 直接构造 App |
| `backend/main.go` → `backend/internal/app/words.go`、`routes.go`、`response.go` | 拆分 | 单词 Handler、路由表、响应工具 | 全部 API 路由、鉴权中间件和 store 接口 |
| `backend/store.go` → `backend/internal/storage/*.go` | 拆分并修改 | 五类 MySQL repository 与 SQL helper | App 的五个 store 接口、模型类型和 `database/sql` |
| `backend/db.go` → `backend/internal/storage/database.go`、`migrations.go` | 拆分并修改 | 去除全局 DB，显式传参 | 入口、管理员初始化、模型合并规则 |
| `backend/models.go` → `backend/internal/model/models.go` | 移动并修改 | 共享结构与纯规则 | app、storage、DeepSeek 解析和迁移逻辑 |
| 其他 `backend/*.go` | 移动 | 按职责进入 `internal/app` | `App`、HTTP 工具、配置缓存、后台任务 |
| 现有 `backend/*_test.go` | 移动/拆分 | 跟随被测 package | 大量直接访问原 package main 的未导出符号 |

## 验证方式

### 验证前置

- **命令 + 预期输出**：

  ```text
  命令：cd backend && go test ./...
  预期：所有 package 均为 ok；原有测试函数没有因迁包被删除或跳过。

  命令：cd backend && go build ./...
  预期：退出码 0，无 import cycle、未定义符号或重复包错误。

  命令：cd backend && go list ./...
  预期：列出 cmd/vocab-server、internal/app、internal/model、internal/storage 四个包。
  ```

- **端到端操作路径（自动化、不开服务）**：

  1. 用 `httptest.NewRequest` 向 `app.Handler()` 发起现有 API 请求。
  2. 请求经过原路由匹配、超时/鉴权中间件和 Handler。
  3. Handler 调用 fake store，并返回与改造前相同的状态码和 JSON。
  4. Handler 调用 fake store，并返回与改造前相同的状态码和 JSON。
  5. 用测试临时目录注入 SPA 静态资源路径，验证未知路径仍进入 SPA fallback，错误请求仍保持原错误结构。

- **边界覆盖**：既有鉴权、禁用用户、错误密码、无数据、无效参数、分页、SRS、DeepSeek/豆包解析、限流和 panic recover 测试全部保留；数据库 schema 与历史数据不变，靠迁移代码逐语句迁移及编译/单测确认；并发语义通过原实现原样迁移和相关测试确认。

### 受影响的测试

CodeGraph 原始输出：

```text
ℹ No test files affected by the changed files.
```

CodeGraph 未跟踪到同包白盒调用；人工确认以下 12 个文件均直接依赖原 `package main` 的未导出符号，全部属于受影响测试：

```text
backend/auth_test.go
backend/deepseek_test.go
backend/dictionary_test.go
backend/doubao_test.go
backend/fakes_test.go
backend/main_test.go
backend/middleware_test.go
backend/models_test.go
backend/normalize_test.go
backend/ratelimit_test.go
backend/settings_test.go
backend/store_test.go
```

- [x] 本项目测试基建可用；改动后需要重跑全部 Go 测试。
- [ ] 本项目无测试基建，跳过。

## 影响面与风险

### 风险点

- `store.go` 返回的词典查询类型当前定义在 `dictionary.go`；必须先把共享查询模型下沉，否则会形成 `app ↔ storage` 循环依赖。
- `db.go` 使用释义合并规则；必须让 storage 单向依赖 model，不能让 model 依赖 storage。
- 当前测试全部是同包白盒测试；实现与测试必须同步迁移，不能靠无意义地公开所有内部符号维持编译。
- `./static` 与音频目录依赖进程工作目录；入口移动后本地和容器仍必须从原工作目录启动。
- 环境变量 helper 当前被多个文件共享；拆包后由 app/storage 各自在所属边界读取或由组合根传值，禁止为复用 helper 建立反向依赖，并保持所有默认值与非法值回退行为。
- 后台取消与等待必须保持为两个独立阶段，不能用一个 `Close()` 改变“取消后台任务 → HTTP Shutdown → 最多等待 10 秒 → 关闭 DB”的原顺序。
- Dockerfile 和文档当前使用 `go build .` / `go run .`，入口移动后必须同步更新。
- 本次为纯重构，任何接口、SQL、默认值或错误文本变化都应视为越界。

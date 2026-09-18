<!-- 阶段记录：模型 GPT-5 (Codex) / 开始 2026-09-18 15:44 CST / 结束 2026-09-18 15:45 CST / 规范版本 @sw/dev-skills@1.1.2 -->

# 将 Go 后端从单包平铺结构重构为显式分包结构

## 需求陈述

当前 `backend/` 的生产代码与测试全部平铺在同一目录、同属 `package main`。需要在不改变接口、数据和运行行为的前提下，建立清晰的启动层、HTTP/业务编排层、数据访问层和领域模型层，让熟悉前端或 Spring Boot 分层的开发者更容易定位职责，也让 Go 编译器能够约束依赖方向。

## 定制 or 通用 ⚠️ 必填

- [x] **通用能力** —— 受益范围：当前项目整个 Go 后端。
- [ ] **单客户定制** —— 不适用；仓库中未发现客户/租户定制目录，本次也不引入客户差异。

`.claude/ai-profile.md` 仍是未配置完成的模板；本次按 CodeGraph 影响面兜底判断。目录扫描只发现 `frontend/src/utils/` 公共候选，与本次后端分包无关。

## 影响面

执行命令：`codegraph impact App`

```text
Impact of changing "App" — 22 affected symbols:

frontend/src/App.vue
  component   App:1

backend/app.go
  struct      App:85
  function    NewApp:110

backend/fakes_test.go
  function    newTestApp:178

backend/auth_test.go
  function    TestHandleLoginSuccess:61
  function    TestHandleLoginRecordsLastLogin:87
  function    TestHandleLoginFailureDoesNotRecordLastLogin:111
  function    TestHandleLoginLockoutAfterThreshold:130
  function    TestHandleChangePasswordClearsOtherSessions:153
  function    TestHandleChangePasswordWrongOldPassword:179
  function    TestHandleResetUserPasswordNotFound:195
  function    TestHandleResetUserPasswordSuccessClearsSessions:209
  function    TestRequireAuthNoHeader:229
  function    TestRequireAuthExpiredSession:242
  function    TestRequireAuthValidSessionPassesThrough:260
  function    TestRequireAdminForbidsNonAdmin:286
  function    TestRequireAdminAllowsAdmin:304
  function    TestHandleLoginRejectsDisabledUser:327
  function    TestRequireAuthRejectsDisabledSession:349
  function    TestHandleDisableUserCannotDisableSelf:371
  function    TestHandleDeleteUserCannotDeleteSelf:387

backend/main.go
  function    main:54
```

- **已确认事实**：后端约 4,080 行生产 Go 代码，所有生产文件和 12 个测试文件当前均在 `package main`；`App`、构造函数、入口与白盒测试直接耦合。
- **已确认事实**：CodeGraph 同名匹配到了 `frontend/src/App.vue`，它与 Go 的 `App` 无调用关系，属于同名噪声。
- **推断**：改动覆盖后端启动、HTTP/业务编排、数据访问、模型和测试五类职责；不触及前端业务模块或跨客户公共层。

## 依赖服务现状

| 依赖 | 当前方法签名 | 现有能力是否满足 | 是否变更契约 | 依据 |
|---|---|---|---|---|
| DeepSeek 查词 | `lookupDeepSeek(context.Context, string, deepseekConfig) ([]Sense, bool, error)` | 是 | 否，只迁移实现位置 | 已确认（`backend/deepseek.go`） |
| 豆包语音 | `synthesizeSpeech(context.Context, string, ttsConfig) ([]byte, error)` | 是 | 否，只迁移实现位置 | 已确认（`backend/doubao.go`） |
| MySQL | `database/sql` 与五类 repository | 是 | 否，只改为显式注入 | 已确认（`backend/store.go`、`backend/db.go`） |

## 核心语义与数据模型

- **关键 ID 的含义** `[已确认]`：用户 ID、单词 ID、`word_key`、会话 token 的来源和含义不变；不改变任何数据库主键、唯一键或接口路径参数。
- **核心数据存哪、怎么流转** `[已确认]`：仍由 MySQL 的 `users`、`sessions`、`words`、`word_dictionary`、`settings` 表持久化；HTTP Handler 经消费方接口调用 storage 实现，后台翻译和 TTS 的写回流程不变。
- **多个入口行为是否一致** `[已确认]`：登录、用户端单词/闪卡/发音接口、管理端用户/设置/词典接口及 SPA fallback 的 method、path、鉴权链、状态码和 JSON 结构均保持不变。
- **启动与关闭顺序** `[已确认]`：连接数据库 → 幂等迁移 → 初始化管理员与历史数据归属 → 加载设置 → 初始化音频目录 → 恢复后台任务 → 启动扫描器/限流清理 → HTTP 服务；关闭时先取消后台任务，再优雅关闭 HTTP，最后等待任务结束。
- **路径语义** `[已确认]`：`./static` 和音频目录继续相对于进程工作目录解析；入口移动后仍从 `backend/`（本地）或 `/app`（容器）启动。
- **并发语义** `[已确认]`：翻译 semaphore、WaitGroup、context 生命周期、登录/密码限流和 repository SQL 原子操作均只迁移归属，不修改算法。

## 设计要点

目标结构：

```text
backend/
├── cmd/vocab-server/main.go       # 唯一组合根：配置、依赖组装、启动和关闭
├── internal/
│   ├── app/                       # 路由、Handler、中间件与现有业务编排
│   ├── storage/                   # database/sql、迁移和 MySQL repositories
│   └── model/                     # 共享领域模型与纯业务规则
├── schema.sql
├── go.mod
└── go.sum
```

- 保留 `net/http`、`database/sql` 和手写 SQL，不引入 Gin、GORM 或依赖注入框架。
- 依赖固定为 `cmd → app`、`cmd → storage`、`app → model`、`storage → model`；`app` 声明自己消费的 store 接口，`storage` 通过 Go 隐式接口实现，禁止 `storage → app` 反向依赖。
- `cmd/vocab-server` 作为 composition root 显式创建数据库连接、repositories 和 `app.App`，替代 `NewApp(*sql.DB)` 内部创建具体 repository。
- 当前 Handler 与业务编排仍放在 `internal/app`，暂不制造只有转发作用的空洞 `service` 层；后续出现跨入口复用或复杂业务规则时再提取。
- `store.go` 在同一个 `storage` package 内按用户、会话、单词、词典、设置拆文件；不为每张表创建一个微型 package。
- DeepSeek、豆包等外部适配器本次先随 `app` 迁移，避免同时改变重试、配置缓存和后台任务生命周期；出现更多供应商或替换需求时再单独抽包。
- 对外共享的类型移动到 `model`；仅属于 HTTP 的 request/response DTO 留在 `app`，避免 storage 反向依赖 HTTP 类型。
- 同包白盒测试随所属实现一起迁移，避免为了兼容旧测试而把内部符号全部公开。
- 入口迁移后同步调整 Docker 构建命令与本地运行文档，不改变最终二进制名、监听端口和容器启动命令。

否决方案：

- 本次不引入 Gin/GORM：它们会扩大行为变化面，且不能自动解决依赖方向问题。
- 本次不直接拆成 Controller/Service/Repository 大量细包：当前业务规模下容易产生样板代码和循环依赖。
- 不只移动文件而保留所有代码在一个 package：那只能改善视觉目录，无法让编译器约束依赖。

## 可复用资产

| 资产类型 | 已发现的候选 | 是否复用 | 不复用的原因 |
|---|---|---|---|
| Store 抽象 | `app.go` 中的 `userStore`、`sessionStore`、`wordStore`、`dictionaryStore`、`settingsStore` | 是 | 不适用 |
| 依赖容器 | `App` 与 `NewApp` | 是，改为显式依赖注入 | 不适用 |
| HTTP 中间件 | `withTimeout`、`recoverMiddleware`、`requireAuth`、`requireAdmin` | 是 | 不适用 |
| 测试替身 | `fakes_test.go` 中的 fake stores 与 `newTestApp` | 是，随 app 包迁移 | 不适用 |
| 数据迁移 | `db.go` 的幂等迁移链 | 是，整体迁入 storage | 不适用 |

## 边界行为清单

- [x] 空 / 无数据场景：列表、查询、翻译空结果保持现有响应，不改变空数组、404 和 204 语义。
- [x] 异常状态：网络失败、权限不足、参数非法、数据库/外部服务异常继续使用原状态码、错误结构与日志策略。
- [x] 历史数据 / 存储结构变更：不修改 schema；现有幂等迁移、历史释义合并、用户归属和时区迁移顺序保持不变。
- [x] 原有 mock / 测试替身 / 本地持久化：fake stores 随包迁移；MySQL、静态文件和音频目录位置不变。
- [x] 接口契约：不新增或变更接口；所有 method、path、JSON 字段、状态码和鉴权保持一致。
- [x] 权限 / 角色 / 客户 / 租户差异：普通用户、管理员、禁用用户的现有行为保持一致；客户/租户维度不适用，因为项目没有该模型。
- [x] 并发、重复请求、幂等性：不改变后台任务并发上限、限流、原子 upsert、重复迁移或优雅关闭逻辑。

## 行为等价的验证方式

- 改动前：`cd backend && go test ./...`，记录现有测试基线；`go build ./...`，确认所有包可编译；记录完整路由表和依赖清单。
- 改动后：执行相同的 `go test ./...` 与 `go build ./...`；测试数量和结果不得减少，路由 method/path/中间件链、`go.mod` 直接依赖、Docker 最终二进制与启动命令必须与改动前等价。
- 不启动 dev server，不做浏览器或线上验收；本次没有 UI 和外部契约变化。

## 验收标准

- [ ] `backend/` 不再是所有源码同属 `package main` 的平铺结构，形成 `cmd/vocab-server`、`internal/app`、`internal/storage`、`internal/model`。
- [ ] `cmd/vocab-server/main.go` 只承担组合根与进程生命周期职责，不包含 Handler 或 SQL。
- [ ] `internal/app` 不直接依赖 `*sql.DB`，SQL 只存在于 `internal/storage`。
- [ ] package 依赖保持单向且不存在 import cycle。
- [ ] 不新增 Gin、GORM、ORM 或 DI 框架依赖。
- [ ] HTTP API、鉴权、数据库结构、后台任务、静态资源和音频路径行为不变。
- [ ] Dockerfile、README、CONTRIBUTING 中的构建/运行入口与新目录一致。
- [ ] 原有测试随代码迁移且不减少；`go test ./...` 与 `go build ./...` 均通过。

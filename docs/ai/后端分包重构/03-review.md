<!-- 适用任务类型：refactor。先查合规，再查影响面与分层。代码质量靠项目自己的 lint 在 CI 里保证，不在此表重复。 -->
<!-- 阶段记录：模型 GPT-5 (Codex) / 开始 2026-09-18 16:06 CST / 结束 2026-09-18 16:17 CST / 规范版本 @sw/dev-skills@1.1.2 -->

# 自审与验收

本次复核覆盖 `git diff`、`git diff --cached`，并逐一纳入 Git 尚未跟踪的 `backend/cmd/**`、`backend/internal/**` 与 `docs/ai/后端分包重构/**`。当前技术栈为 Go Modules、`net/http`、`database/sql`、MySQL；前端 Vue/Vite 代码不在本次改动范围。

## 第一步：是否符合上游产物

- [x] `01-spec.md` 的验收标准已逐条核对
- [x] `02-plan.md` 的改动清单全部完成
- [x] 没有未说明的计划外改动
- [x] 「通用能力」判定与实际落点一致
- [x] `02b-design.md` 涉及的包边界、依赖注入、模型、迁移、启动关闭及测试设计均已核对

### 验收标准逐条核对

| 验收标准 | 结果 | 依据 |
|---|---|---|
| `backend/` 形成 `cmd/vocab-server`、`internal/app`、`internal/storage`、`internal/model` | 达成 | `go list ./...` 精确列出四个包；`backend/` 根目录已无 `.go` 文件 |
| `cmd/vocab-server/main.go` 只负责组合根与进程生命周期 | 达成 | 文件只组装 DB/repository/app、启动 HTTP、处理信号与关闭；无 Handler 或 SQL |
| `internal/app` 不直接依赖 `*sql.DB`，SQL 只存在于 `internal/storage` | 达成 | app 生产代码无 `*sql.DB`、查询或迁移 SQL；repository 由 `Dependencies` 注入 |
| 包依赖单向且无 import cycle | 达成 | `app → model`、`storage → model`，仅 `cmd` 同时导入 app/storage；`go list` 与构建均通过 |
| 不新增 Web/ORM/DI 框架 | 达成 | `backend/go.mod`、`go.sum` 无改动，无 Gin/GORM/Chi/Fiber/Echo/sqlc/DI 框架 |
| HTTP、鉴权、数据库、后台任务、静态资源和音频路径行为不变 | 达成 | 32 条 API pattern 差异为空；中间件嵌套相同；141 个 JSON tag 与 89 个包含 SQL 关键字的反引号字面量集合一致；环境变量默认值、迁移顺序及关闭顺序逐项核对一致 |
| Dockerfile 与开发/部署文档使用新入口和新路径 | 达成 | 构建改为 `./cmd/vocab-server`；README、CONTRIBUTING、DEPLOYMENT 的入口、分层和迁移引用已同步 |
| 原测试不减少，完整测试与编译通过 | 达成 | 原 70 个测试函数全部保留，现有 73 个；`go test -count=1 ./...` 与 `go build ./...` 均通过 |

### 计划与详细设计核对

- `02-plan.md` 的 13 项改动均有对应实现和验证；实现期间新增的 `env.go`、`lifecycle.go`、`model_aliases.go`、`response_test.go` 已回补进计划。
- 五个 Store 接口由 app 声明，五个 MySQL repository 在 storage 实现，并由唯一组合根注入。
- 领域类型和纯规则迁至 model；HTTP 分页信封仍在 app；新旧 JSON tag 多重集完全一致。
- `Migrate(db)`、`FinalizeWordsUserID(db, adminID)` 和应用初始化的顺序与设计一致；关闭仍为“取消后台任务 → HTTP Shutdown → 限时等待任务 → 关闭 DB”。
- 路由契约测试已补齐一条带有效 Session 的完整调用链，并保留 32 条 API pattern 与 SPA fallback 覆盖。

偏差说明：

| 偏差 | 原因与闭环 |
|---|---|
| 初版计划漏列 4 个 app 拆分文件 | 文件均服务既定设计，无范围扩张；复核期间已回补 `02-plan.md` |
| 初版 CONTRIBUTING 仍引用旧平铺文件和 `migrateSchema()` | 复核期间已全部改为 cmd/internal 新结构和 `storage.Migrate(db)` |
| 初版路由测试只验证未登录请求停在鉴权层 | 已新增 `TestHandlerServesAuthenticatedMeThroughFullStack`，验证有效 Session、真实 Handler 响应和续期 |

## 影响面复核

执行：`codegraph impact App`

```text
Impact of changing "App" — 87 affected symbols:

backend/internal/app/app.go
  struct      App:98
  function    New:124

backend/internal/app/pronunciation.go
  method      spawnTTS:28
  method      synthesizeAndSave:51
  method      handlePronounce:70

backend/internal/app/words.go
  method      translateAndSave:186
  method      spawnTranslation:116
  method      handleReviewColors:448
  method      handleAddWord:27
  method      tryIncrementExisting:240
  method      resumeStuckTranslations:140
  method      startStuckTranslationSweeper:156
  method      sweepStuckTranslations:171
  method      saveWordSenses:227
  method      handleListWords:327
  method      handleResetReviewCounts:352
  method      handleListTranslatingWords:365
  method      handleLookupWord:388
  method      handleWordStats:416
  method      handleFlashcardQueue:453
  method      handleFlashcardReview:465
  method      handleDeleteWord:522
  method      handleArchiveWord:543
  method      handleUnarchiveWord:547
  method      setWordArchived:552
  method      handleRetryWord:575
  method      handleSetWordImportant:622

backend/internal/app/settings.go
  method      loadSettings:76
  method      seedSettingIfMissing:109
  method      refreshSettingsCache:115
  method      handleUpdateSettings:267
  method      getDeepSeekConfig:153
  method      maskedSettingsView:228
  method      handleGetSettings:249
  method      getTTSConfig:159
  method      getReviewColorConfig:165

backend/internal/app/lifecycle.go
  method      Initialize:12
  method      CancelBackground:26
  method      WaitBackground:31

backend/cmd/vocab-server/main.go
  function    main:24

backend/internal/app/dictionary.go
  method      handleRetryDictionary:114
  method      upsertDictionaryOccurrence:16
  method      lookupDictionarySenses:23
  method      saveDictionarySenses:40
  method      handleVocabularyIndex:67
  method      handleListDictionary:80
  method      handleExportDictionary:151
  method      handleDeleteDictionaryEntry:173
  method      handleDeleteDictionaryBatch:192

backend/internal/app/routes.go
  method      Handler:19

backend/internal/app/routes_test.go
  function    TestHandlerRegistersExistingAPIRoutes:16
  function    TestHandlerServesAuthenticatedMeThroughFullStack:74
  function    TestHandlerFallsBackToSPAIndex:107

backend/internal/app/middleware.go
  function    recoverMiddleware:25

backend/internal/app/middleware_test.go
  function    TestRecoverMiddlewareRecoversPanic:10

backend/internal/app/auth.go
  method      BootstrapAdmin:41
  method      createUser:68
  method      handleCreateUser:423
  method      handleResetUserPassword:93
  method      handleDisableUser:128
  method      handleDeleteUser:165
  method      handleChangePassword:202
  method      handleLogin:261
  method      handleLogout:321
  method      handleMe:340
  method      requireAuth:367
  method      requireAdmin:407
  method      handleListUsers:452

backend/internal/app/auth_test.go
  function    TestHandleResetUserPasswordNotFound:195
  function    TestHandleResetUserPasswordSuccessClearsSessions:209
  function    TestHandleDisableUserCannotDisableSelf:371
  function    TestHandleDeleteUserCannotDeleteSelf:387
  function    TestHandleChangePasswordClearsOtherSessions:153
  function    TestHandleChangePasswordWrongOldPassword:179
  function    TestHandleLoginSuccess:61
  function    TestHandleLoginRecordsLastLogin:87
  function    TestHandleLoginFailureDoesNotRecordLastLogin:111
  function    TestHandleLoginLockoutAfterThreshold:130
  function    TestHandleLoginRejectsDisabledUser:327
  function    TestRequireAuthNoHeader:229
  function    TestRequireAuthExpiredSession:242
  function    TestRequireAuthValidSessionPassesThrough:260
  function    TestRequireAuthRejectsDisabledSession:349
  function    TestRequireAdminForbidsNonAdmin:286
  function    TestRequireAdminAllowsAdmin:304

backend/internal/app/fakes_test.go
  function    newTestApp:178

frontend/src/App.vue
  component   App:1
```

- [x] 所有真实调用方均已确认，没有漏改
- [x] 实际影响面与 `01-spec.md` 预估一致

CodeGraph 的数量由设计阶段 22 增至 87，是因为分包后索引识别到了 `App` 的全部接收者方法及新增路由测试，并非业务范围扩大；`frontend/src/App.vue` 仍是同名噪声，与 Go 后端无调用关系。

### 分层检查 ⚠️

- [x] 改动未下沉到跨客户公共层
- [ ] 改动触及公共层，理由：不适用

`.claude/ai-profile.md` 仍是未确认的模板，因此按 CodeGraph 与目录结构兜底。本仓库没有客户/租户定制目录，本次是当前项目整个后端的通用分层；`frontend/src/utils/` 候选公共层未改动。

`internal/app` 仍为保持既有错误分支而识别 `sql.ErrNoRows` 与 MySQL 1062，但没有持有 `*sql.DB`、执行 SQL 或导入 `internal/storage`。这不违反本次明确验收标准；若未来要替换数据库实现，可另行把这些错误映射成领域错误。

### 注释与可读性 ⚠️ lint 查不到，人工 review 容易漏

- [x] 关键方法有关键注释（做什么 + 为什么）
- [x] 较大流程的关键文件顶部有文件职责与调用链说明

入口组合根、路由装配、单词后台流程、模型叶子包和 storage 包均写明职责/依赖方向；迁移、初始化、取消与限时等待等关键方法说明了顺序约束。

## 验证记录

| 验证项 | 实际结果 | 通过 |
|---|---|---|
| 改造前 `cd backend && go test ./...` | 实施阶段实际执行：`ok vocab-backend` | ✅ |
| 改造前 `cd backend && go build ./...` | 实施阶段实际执行：退出码 0 | ✅ |
| 改造后 `cd backend && go test -count=1 ./...` | reviewer 独立执行：cmd 无测试；app/model/storage 均 `ok` | ✅ |
| 改造后 `cd backend && go build ./...` | reviewer 独立执行：退出码 0，无输出 | ✅ |
| `cd backend && go list ./...` | 精确列出 `cmd/vocab-server`、`internal/app`、`internal/model`、`internal/storage` | ✅ |
| 路由完整链路定向测试 | 3 个 Handler 契约测试及全部 32 个 API 子用例通过；有效 Session 用例验证 JSON 与续期 | ✅ |
| 原测试保留 | 旧 70 个测试名称无删除；新增 3 个，现共 73 个 | ✅ |
| 路由、JSON、SQL 静态等价核对 | 32 条 API pattern 无差异；141 个 JSON tag 无差异；89 个包含 SQL 关键字的反引号字面量无差异 | ✅ |
| `gofmt -d` | 无输出 | ✅ |
| `git diff --check` + 未跟踪文件逐个 `--no-index --check` | 无空白错误输出 | ✅ |

## 未验证项与剩余风险

| 未验证/风险项 | 为什么现在验不了 | 补验方式 + 负责人 + 时间 |
|---|---|---|
| 真实 MySQL 上执行全部迁移与 repository SQL | 本次确认的验收边界是静态等价、单测和编译，不启动服务或外部数据库 | 下次实际发布前由后端维护者在可回滚的测试库执行启动迁移与核心查询回归 |
| 真实进程信号与容器启动 | 本次明确不启动 dev server、不部署；当前以调用顺序静态核对和编译证明 | 下次部署前由发布负责人验证容器启动、SIGTERM 优雅关闭和健康状态 |

以上均不是 `02-plan.md` 的未执行验证前置，不影响本次“通过”结论。

## 测试

- [x] 已迁移原 70 个测试，并新增：`backend/internal/app/routes_test.go` 的 3 个路由/完整链路/SPA fallback 测试
- [x] 已保留 storage helper、模型规则、鉴权、限流、外部响应解析等既有覆盖
- [ ] 该改动落在「不要求测试」档
- [ ] 该改动落在「要求测试」档但未补

## 遗留事项

- `internal/app` 对 `sql.ErrNoRows` 与 MySQL 1062 的识别是为保持既有 HTTP 语义而保留的技术耦合；若未来替换 MySQL，再单独设计存储错误到领域错误的映射，本次不扩张重构范围。
- 当前 `backend/cmd/**`、`backend/internal/**` 和 `docs/ai/后端分包重构/**` 仍为未跟踪文件；提交时必须显式纳入，不能只使用会忽略未跟踪文件的提交方式。

## 结论

- [x] **通过**：无 critical，验证前置全部实际执行并达标，产物链完整
- [ ] **有条件通过**：验证前置有未执行项
- [ ] **阻塞**：存在 critical 或关键产物缺失

本次重构符合确认范围：保留 `net/http`、`database/sql` 与手写 SQL，未修改 API、数据库结构或业务语义。可以进入提交阶段。

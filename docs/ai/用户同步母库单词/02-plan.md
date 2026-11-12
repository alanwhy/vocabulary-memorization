# 实现计划

阶段记录：Codex（GPT-6，具体子型号未提供）；2026-11-12 10:00（Asia/Shanghai）；规范 @sw/dev-skills@1.3.3。执行模式见 [01-spec.md](01-spec.md)。

## 背景

feature：为用户管理页增加母库全量补齐入口，需求与验收见 01。

## 验证方式

| 命令/操作路径 | 预期结果 |
|---|---|
| 管理员进入用户管理 → 选择用户同步 → 用户登录查看个人词库 | 缺词新增为 1 次，重复词原次数和归档状态保留，管理页显示新总数 |
| 同一用户重复同步/两次并发同步 | 不产生重复词，不增加原词次数 |
| 普通账号、非法 ID、不存在用户、数据库失败 | 接口拒绝或明确报错；页面恢复按钮 |
| `cd backend && go test ./...` | 后端兼容性及新路由契约通过；真实数据库用例无专用 DSN 时明确跳过 |
| `cd frontend && pnpm exec eslint src/components/admin/AdminUsers.vue && pnpm build` | 新组件静态检查及构建通过 |

受影响测试：`codegraph affected backend/internal/app/routes.go` 返回 “No test files affected”；现有后端有 22 个 Go 测试文件，可用 `go test`。前端无 UI 自动化用例，以构建和手动验收为准。新增后端路由契约测试；核心数据库语义增加显式隔离库用例，不访问生产数据。

## 改动清单

- [x] `backend/internal/app/app.go` —— WordStore 新增事务同步方法签名 —— 后端编译测试通过。
- [x] `backend/internal/storage/word_sync.go` —— 验证用户、批量补齐、统计及提交 —— 已实现；真实数据库效果待用户验收。
- [x] `backend/internal/app/user_words.go`、`backend/internal/app/routes.go` —— 管理员同步接口及错误映射 —— 新路由测试通过。
- [x] `frontend/src/components/admin/AdminUsers.vue` —— 按行按钮、确认提示、加载状态、单词数反馈 —— lint/构建通过；页面待手动验收。
- [x] `backend/internal/app/user_words_test.go`、`backend/internal/storage/word_sync_test.go` —— 鉴权、参数、结果、事务业务覆盖 —— 路由测试通过，隔离库测试因无 DSN 跳过。
- [x] `docs/ai/用户同步母库单词/03-review.md` —— 记录实际验证和用户验收项 —— 按用户要求未独立复核。

顺序：接口设计 → 后端持久化及路由 → 页面 → 必要验证及记录。

## 关键文件

AdminView 渲染 AdminUsers；AdminUsers 复用 apiPost 调用 Handler 管理员路由；新 handler 调用 WordStore.SyncDictionaryToUser，由 WordRepository 执行事务。详见 [02b-design.md](02b-design.md)。

## 本次边界

不提交、发布或归档；不改同义替换模块，不触发母库全量查词，不增加 UI 自动化或独立复核。

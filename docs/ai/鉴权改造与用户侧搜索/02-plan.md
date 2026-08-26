<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-26 08:00 / 结束 2026-08-26 08:12 / 本阶段 token（in/out）留空（无 /cost 命令）/ 本阶段成本 留空（无 /cost）/ 规范版本 team-dev-skills@1.0.0 -->

# 实现计划

## 背景

- **任务类型**：feature
- **一句话做什么**：接口鉴权从 cookie 改为 Bearer Token，用户侧新增单词快速搜索（支持中英文模糊 + 无释义筛选）、个人中心新增重置次数按钮，分页改为 20 条，并复核删除二次确认。

## 改动方案

### 待拍板事项（对齐清单）⚠️

| 事项 | 结论状态 | 结论 / 暂缓原因 + 时间点 | 阻塞的后续任务 |
|---|---|---|---|
| 重置次数是否**含已归档单词** | 未结论 | 暂按「含已归档」（`WHERE user_id=?` 不限定 archived）实现；若浩原只要活跃词，改为 `AND archived=0` 一行即可。验收前请确认。 | 第 4 / 9 项的重置次数实现（不阻塞开始，仅语义待定） |

### 改动清单

- [x] `backend/auth.go` —— 新增 `bearerToken(r)` 助手；`handleLogin` 改返回 `{ token, user }` 且不再 `SetCookie`；`handleLogout` / `handleChangePassword` / `requireAuth` 从 `r.Cookie` 改读 `Authorization: Bearer` 头 —— 验证：`cd backend && go build ./...` 通过
- [x] `backend/auth_test.go` —— 改造 7 个鉴权用例为 Bearer 语义（`TestHandleLoginSuccess` 断言响应体含 token、`TestRequireAuthNoCookie` 改为无 Authorization 头、其余 `AddCookie` 改设 `Authorization` 头）；新增 `bearerToken` 纯函数测试 —— 验证：`cd backend && go test ./...` 全绿（依赖第 1 项）
- [x] `backend/app.go` + `backend/store.go` —— `wordStore` 接口的 `ListPage`/`CountByUser` 增加 `keyword, status` 参数，新增 `ResetReviewCounts`；`dictFilterWhere` 改名 `senseFilterWhere`（同步改 `dictionaryRepo.ListPage`/`Count` 两处调用），`wordRepo.ListPage`/`CountByUser` 复用该 helper —— 验证：`cd backend && go build ./...` 通过
- [x] `backend/main.go` —— `handleListWords` 读取 `keyword`/`status` 透传给 store；新增 `handleResetReviewCounts` 与路由 `POST /api/words/reset-counts` —— 验证：`cd backend && go build ./...` 通过，`curl -s -X POST localhost:8080/api/words/reset-counts`（无头）返回 401（依赖第 3 项）
- [x] `backend/store_test.go` —— 新增 `senseFilterWhere` 纯函数测试（keyword 空/命中、status 三种取值、组合）—— 验证：`cd backend && go test ./...` 全绿（依赖第 3 项）
- [x] `frontend/src/api/client.js` + `frontend/src/stores/auth.js` —— `request()` 从 localStorage（key `vocab_token`）读 token 拼 `Authorization: Bearer` 头，移除 `credentials: 'include'`，401 时清 token；`login` 存 token+user、`logout` 清 token、`checkAuth` 按 token 有无决定是否调 `/api/me` —— 验证：浏览器登录成功、刷新仍登录、登出后回登录框（依赖第 1 项）
- [x] `frontend/src/composables/usePaginatedList.js` —— `PAGE_SIZE` 由 100 改 20 —— 验证：首页 Network 里 `/api/words?...limit=20`，每批 20 条（可与第 6 项并行）
- [x] `frontend/src/views/HomeView.vue` —— 新增搜索框（keyword）+「暂无释义」筛选（status），300ms 防抖后带参数重查 —— 验证：输入英文/中文子串命中、选「暂无释义」只显示无释义词（依赖第 4、6 项）
- [x] `frontend/src/views/ProfileView.vue` —— 新增「重置次数」按钮，`ElMessageBox.confirm` 二次确认后调 `POST /api/words/reset-counts` —— 验证：确认后回首页所有词 review_count=1、SRS 排期不变（依赖第 4、6 项）
- [x] `frontend/src/components/admin/AdminDictionary.vue` —— 「导出 CSV」由 `<a href>` 改为 `fetch` + Blob 下载（带 Authorization 头）—— 验证：点导出能正常下载 CSV（依赖第 6 项）
- [x] 全量回归 —— `cd backend && go test ./...`；`cd frontend && npm run lint && npm run build`；并按「验证前置」手工走查一遍 —— 验证：全绿 + 手工走查通过（依赖以上全部）

### 本次不做

- 不把鉴权升级为无状态 JWT（现成 `sessions` 表 + 滑动过期已够，JWT 引入新依赖且失去服务端吊销能力）。
- 不删 `Config.CookieSecure` / `.env` 的 `COOKIE_SECURE` 配置项（改动配置面属范围扩散，留着无害）。
- 归档页（ArchiveView）不加搜索（需求只提「用户侧快速搜索」，落在首页活跃列表）。
- 删除二次确认现状已满足，不改代码，只做验收复核。
- 不新增表结构、不迁移数据。

## 关键文件

| 文件 | 改动性质 | 具体改哪个函数/类型/模式 | 谁调用它 / 它影响谁（callers/callees 摘要） |
|---|---|---|---|
| `backend/auth.go` | 修改 | `bearerToken`（新增）、`handleLogin`、`handleLogout`、`handleChangePassword`、`requireAuth` | `requireAuth` 被 `requireAdmin`（auth.go:316）+ `main` 路由装配 + 3 个测试调用；callees：`sessions.FindWithUser`/`Touch`、`writeError`、`sessionCookieName`。`handleLogin` 被 4 个测试覆盖 |
| `backend/app.go` | 修改 | `wordStore` 接口（`ListPage`/`CountByUser` 签名、新增 `ResetReviewCounts`） | 由 `wordRepo`（store.go）实现，`NewApp` 装配；改签名后 `wordRepo` 必须同步，Go 编译期兜底 |
| `backend/store.go` | 修改 | `dictFilterWhere`→`senseFilterWhere`、`wordRepo.ListPage`/`CountByUser`、`wordRepo.ResetReviewCounts`（新增） | `senseFilterWhere` 被 `dictionaryRepo.ListPage`/`Count` 调用；`wordRepo.ListPage`/`CountByUser` 被 `handleListWords`（经接口）调用 |
| `backend/main.go` | 修改 | `handleListWords`、`handleResetReviewCounts`（新增）、路由注册 | `handleListWords` callees：`currentUser`/`parsePagination`/`words.CountByUser`/`words.ListPage`/`writeJSON`/`newPageResult`；无外部调用方 |
| `backend/auth_test.go` | 修改 | 7 个鉴权用例 + `bearerToken` 测试（新增） | 覆盖 `requireAuth`/`requireAdmin`/`handleLogin`/`handleChangePassword` 的 cookie→Bearer 语义 |
| `backend/store_test.go` | 修改 | `senseFilterWhere` 测试（新增） | 纯函数测试，无外部依赖 |
| `frontend/src/api/client.js` | 修改 | `request()` 加 Authorization 头、去 credentials、401 清 token | 被 `auth.js` store + 各视图的 `apiGet/apiPost/apiDelete/apiPut` 调用 |
| `frontend/src/stores/auth.js` | 修改 | `login`/`logout`/`checkAuth` 的 token 存取 | 被 `LoginForm.vue`、`router/index.js`（beforeEach）、`App.vue` 调用 |
| `frontend/src/composables/usePaginatedList.js` | 修改 | `PAGE_SIZE` 常量 | 被 `HomeView`/`ArchiveView`/`AdminDictionary` 3 处调用 |
| `frontend/src/views/HomeView.vue` | 修改 | 搜索框 + 状态筛选 + 请求参数 | 调用 `usePaginatedList`、`useWordActions`、`useTranslatingPoll` |
| `frontend/src/views/ProfileView.vue` | 修改 | 重置次数按钮 | 调用 `apiPost`、`useAuthStore` |
| `frontend/src/components/admin/AdminDictionary.vue` | 修改 | 导出 CSV 逻辑 | 调用 `apiGet`/`apiDelete`/`apiPost` |

## 验证方式

### 验证前置 ⚠️ 必填，写代码之前填

**命令验证**：

```
命令：cd backend && go test ./...
预期：全部 PASS，无 FAIL
命令：cd frontend && npm run lint && npm run build
预期：无 lint 报错，构建成功产出 dist/
```

**端到端操作路径验证（覆盖边界行为清单）**：

1. **鉴权**：`curl -s http://localhost:8080/api/words`（不带 Authorization 头）→ 返回 `401 {"error":"请先登录"}`；`curl -s -X POST localhost:8080/api/login -H 'Content-Type: application/json' -d '{"username":"<账号>","password":"<密码>"}'` → 返回体含 `token` 与 `user`，且响应头**无 `Set-Cookie`**；再用返回的 token 带 `Authorization: Bearer <token>` 请求 `/api/words` → 200 返回 items。
2. **登录态持久化**：浏览器登录 → 手动刷新页面 → 仍是登录态（token 已存 localStorage）；点登出 → 回到登录框，localStorage 的 `vocab_token` 被清除。
3. **分页**：首页滚动到底 → Network 面板里 `/api/words` 请求带 `limit=20`，每次追加 20 条。
4. **搜索（含边界）**：首页搜索框输入英文子串（如 `app`）→ 命中单词；输入中文子串（如 `苹果`）→ 命中释义；选「暂无释义」→ 列表只剩 `senses` 为空的词；三者可与排序叠加；清空关键字 → 恢复全量。
5. **重置次数**：个人中心点「重置次数」→ 弹二次确认 → 取消无变化 → 确认后回首页，所有词 `review_count` 徽标回到 ×1，闪卡排期（下次到期时间）不变。
6. **删除二次确认**：首页删词、词库管理删单条/批量 → 均弹确认 → 取消不删、确认才删。
7. **导出 CSV**：词库管理点「导出 CSV」→ 正常下载可打开的 CSV 文件（Bearer 鉴权下不 401）。

### 受影响的测试

```
（codegraph affected 后端三个文件返回「No test files affected」，但其启发式未识别同包的 _test.go；
   以下为人工核对到的受影响测试：）
- backend/auth_test.go：TestHandleLoginSuccess / TestRequireAuthNoCookie / TestRequireAuthExpiredSession
  / TestRequireAuthValidSessionPassesThrough / TestRequireAdminForbidsNonAdmin / TestRequireAdminAllowsAdmin
  / TestHandleChangePasswordClearsOtherSessions
- backend/store_test.go：新增 senseFilterWhere 测试（纯函数）
```

- [x] 上述测试改动后需要重跑
- [ ] 本项目无测试基建，跳过（不适用：本项目有 `go test` 测试基建）

## 影响面与风险

> 影响面见 `01-spec.md` 的「影响面」节，本阶段无增量。

### 风险点

- **导出 CSV 的 `<a href>` 会随 Bearer 切换失效**：已识别并纳入第 10 项，改为 fetch + Blob；漏改会导致导出 401。
- **auth_test.go 有 7 个用例从 cookie 改 Bearer**：漏改任一都会 `go test` 红，需与第 1 项同批完成。
- **`dictFilterWhere` 改名会动 dictionary.go 两处调用**：改名前 `grep dictFilterWhere` 确认调用点无遗漏。
- **localStorage 存 token 有 XSS 暴露面**：个人项目可接受，已记录；如需更严可后续换内存态 + 刷新重登。

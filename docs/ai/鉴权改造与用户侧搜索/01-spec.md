<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-26 07:47 / 结束 2026-08-26 08:00 / 本阶段 token（in/out）留空（无 /cost 命令）/ 本阶段成本 留空（无 /cost）/ 规范版本 team-dev-skills@1.0.0 -->
<!-- 本文的「影响面 / 依赖服务现状 / 核心语义与数据模型 / 可复用资产」都是现状（已确认的事实），和「设计要点」分开写：现状只写确认过的事实，没确认的标「推断」或「待确认」，别拿推测当事实。 -->

# 接口改 Bearer 鉴权 + 用户侧快速搜索 + 个人中心重置次数

## 需求陈述

浩原要在一批用户体验与安全层面的改动：

1. 分页每页从 100 条改为 20 条。
2. 接口鉴权从现在的 cookie 改成 Bearer Token（Authorization 头），消除「直接访问 url 也能拿到数据」的顾虑。
3. 用户侧（首页）新增快速搜索：支持按英文单词、中文释义做模糊搜索，且能单独筛出「没有查到释义」的单词。
4. 个人中心新增「重置次数」按钮，把当前用户所有单词的次数（review_count）重置为 1，需二次确认。
5. 用户侧与管理侧任何删除都需要二次确认。

## 定制 or 通用 ⚠️ 必填

- [x] **通用能力** —— 受益范围：本项目全部用户。

本项目是单租户个人项目（背单词应用），没有「多客户 / 定制层」的概念，所有改动都是对产品本身的通用增强，不存在「给某个客户加需求动了公共代码」的风险。`.claude/ai-profile.md` 仍是模板未填写，但无物理多客户边界，此判定不受影响。

## 影响面

`codegraph impact` 对核心符号的原始输出：

```
Impact of changing "requireAuth" — 8 affected symbols:
backend/auth.go
  method      requireAuth:285
  method      requireAdmin:316
backend/auth_test.go
  function    TestRequireAdminForbidsNonAdmin:260
  function    TestRequireAdminAllowsAdmin:278
  function    TestRequireAuthNoCookie:203
  function    TestRequireAuthExpiredSession:216
  function    TestRequireAuthValidSessionPassesThrough:234
backend/main.go
  function    main:54

Impact of changing "handleListWords" — 1 affected symbols:
backend/main.go
  method      handleListWords:437

Impact of changing "handleLogin" — 5 affected symbols:
backend/auth.go
  method      handleLogin:182
backend/auth_test.go
  function    TestHandleLoginSuccess:38
  function    TestHandleLoginRecordsLastLogin:61
  function    TestHandleLoginFailureDoesNotRecordLastLogin:85
  function    TestHandleLoginLockoutAfterThreshold:104
```

- **已确认事实**：上面的 codegraph 原始输出。
- **推断**：Bearer 改造会牵连 `requireAuth` / `requireAdmin` / `handleLogin` / `handleLogout` / `handleChangePassword` 五个后端符号及 `auth_test.go` / `main_test.go` 里的鉴权相关用例；搜索改动只动 `handleListWords` 与 `wordRepo` 两个符号。整体约 3 个后端文件 + 4 个前端文件，无多客户模块，不存在「公共层污染」问题。

## 依赖服务现状 ⚠️ 涉及外部服务 / 公共层时必填

| 依赖（公共方法） | 方法签名 | 现有方法是否满足 | 是否需要新增/变更 | 依据 |
|---|---|---|---|---|
| `wordRepo.ListPage` | `ListPage(ctx, userID, archived, sort, limit, offset)` | 否，无 keyword/status 过滤 | 是，加 keyword/status 参数 | 已确认（读了 [store.go:294](backend/store.go#L294)） |
| `wordRepo.CountByUser` | `CountByUser(ctx, userID, archived)` | 否，无过滤 | 是，加 keyword/status 参数 | 已确认（读了 [store.go:306](backend/store.go#L306)） |
| `dictFilterWhere` | `dictFilterWhere(keyword, status)` | 是，word_key + JSON_SEARCH 释义 + status 过滤逻辑现成 | 复用（抽成共享或对 words 表复制同构逻辑） | 已确认（读了 [store.go:637](backend/store.go#L637)） |
| `sessionRepo` | `Create/FindWithUser/Touch/DeleteByToken/DeleteByUser/DeleteByUserExcept` | 是，Bearer 只换 token 的传输方式，会话表不动 | 否 | 已确认（读了 [store.go:144](backend/store.go#L144)） |
| `usePaginatedList` | `usePaginatedList(fetchPage, {pageSize})` | 是，PAGE_SIZE 常量改一处即可 | 否 | 已确认（读了 [usePaginatedList.js:3](frontend/src/composables/usePaginatedList.js#L3)） |

不涉及外部 Dubbo/HTTP 服务，本表只列内部公共方法。

## 核心语义与数据模型 ⚠️ 必填

- **「次数」的含义** `[已确认]`：`words.review_count`（INT，默认 1），即单词累计背诵次数。个人中心「重置次数」= 把所有 `review_count` 置回 1，**不动** SRS 排期字段（`due_at` / `interval_days` / `ease_factor`）与 `last_reviewed_at`（用户已明确「只重置 review_count」）。
- **鉴权 token 的存储与流转** `[已确认]`：token 是 64 位 hex 随机串，存 `sessions` 表（服务端会话，滑动过期 30 天）。改 Bearer 后 token 生成与存储不变，只把「从 cookie 读」换成「从 `Authorization: Bearer <token>` 头读」；登录响应从返回 `user` 改为返回 `{ token, user }`，前端把 token 存 localStorage 并在每次请求统一加头。
- **搜索数据源** `[已确认]`：用户侧搜**当前用户自己的单词**（`words` 表，`user_id = 当前用户`），不是全局词库 `word_dictionary`。
- **「没有查到释义的单词」** `[已确认]`：指 `words.senses` 为空（`senses IS NULL OR JSON_LENGTH(senses)=0`）的单词，用户要能**单独筛出**这批词（等价于管理端 `status=no_definition` 过滤），而非仅「英文搜索能命中」。
- **多个入口行为是否一致** `[已确认]`：搜索加在首页（HomeView）的活跃列表上，复用管理端 `keyword + status` 的过滤语义；归档页（ArchiveView）本次不加搜索，保持行为独立。删除确认、鉴权对「用户侧 / 管理侧」两个入口语义一致。
- **和数据相关的边界情况** `[推断]`：重置次数对「含已归档的词」是否生效——需求原文是「用户当前的所有单词」，我按**含已归档**处理（`WHERE user_id = ?` 不额外限定 archived）；若浩原只想重置活跃词，改成 `AND archived = 0` 即可，属一句话级别调整，已列入验收前确认项。

## 设计要点

1. **Bearer Token 鉴权（替换 cookie）**：登录成功返回 `{ token, user }`，不再 `SetCookie`；`requireAuth` 改读 `Authorization: Bearer` 头；`handleLogout` / `handleChangePassword` 里「当前会话 token」的获取同样改从请求头读。token 仍走 `sessions` 表 + 滑动过期，服务端吊销能力保留。
   - **否掉方案：改成无状态 JWT**——现成的 `sessions` 表 + `randomToken` + 滑动过期已满足需求，JWT 要引新依赖且失去服务端主动吊销能力（改密/重置密码后立即失效做不到），收益不匹配成本。

2. **导出 CSV 链接会失效，必须一并改**：管理端「导出 CSV」现在是纯 `<a href="/api/admin/dictionary/export">`（[AdminDictionary.vue:136](frontend/src/components/admin/AdminDictionary.vue#L136)），靠 cookie 鉴权。改 Bearer 后 `<a href>` 不带 Authorization 头，会 401。改成 `fetch` + `Blob` 下载（显式带 Authorization 头）。

3. **分页 20 条**：后端 `defaultPageLimit` 已是 20（[main.go:392](backend/main.go#L392)），是前端 `usePaginatedList.js` 的 `PAGE_SIZE = 100` 显式传参覆盖成 100。只改前端 `PAGE_SIZE = 20` 一处，首页 / 归档 / 词库管理三个列表同时生效。

4. **用户侧搜索**：`/api/words` 增加 `keyword`、`status` 两个查询参数，过滤逻辑与管理端 `dictFilterWhere` 同构（`word_key LIKE ? OR JSON_SEARCH(senses, 'one', ?, '$.translation') IS NOT NULL` + `status` 按 `senses` 有无），并叠加 `user_id = ? AND archived = ?`。为复用，把 `dictFilterWhere` 抽成一个可同时服务两张表（列名相同）的共享 helper。

5. **重置次数**：新增 `POST /api/words/reset-counts`（requireAuth），`UPDATE words SET review_count = 1 WHERE user_id = ?`，返回 204。个人中心加「重置次数」按钮，`ElMessageBox.confirm` 二次确认。

6. **删除二次确认**：现状已全部具备（用户删词 [useWordActions.js:34](frontend/src/composables/useWordActions.js#L34)、管理删词库单条/批量 [AdminDictionary.vue:67](frontend/src/components/admin/AdminDictionary.vue#L67)），**无需改代码**，验收时复核一遍无遗漏即可。

接口与前端细节（登录契约、token 存取、搜索请求参数、导出 blob 下载）落到 `02b-design.md`，本文件只写决策与理由。

## 可复用资产 ⚠️ 建表 / 建实体 / 建接口类任务必填

| 资产类型 | 已发现的候选 | 是否复用 | 不复用的原因 |
|---|---|---|---|
| 过滤 SQL 逻辑 | `dictFilterWhere`（[store.go:637](backend/store.go#L637)） | 是 | words 表与 word_dictionary 表的 `word_key`/`senses` 列名一致，抽共享 helper 直接复用 |
| 分页状态机 | `usePaginatedList` | 是 | 改 `PAGE_SIZE` 常量即可，逻辑不动 |
| 二次确认弹窗 | `ElMessageBox.confirm`（element-plus） | 是 | 现有删词/删词库已用，重置次数按钮照搬 |
| 鉴权中间件 | `requireAuth` / `requireAdmin` | 是 | 只改 token 读取方式，中间件骨架不动 |
| 会话存储 | `sessionRepo`（`sessions` 表） | 是 | Bearer 只换传输方式，表结构与 CRUD 全复用 |

## 边界行为清单 ⚠️ 必填

- [x] 空 / 无数据场景：搜索无匹配 → `items=[]`、`total=0`、`has_more=false`，前端展示「没有匹配的单词」；重置次数时用户无任何单词 → `UPDATE` 影响 0 行，返回 204，前端提示「已重置」不报错。
- [x] 异常状态（网络失败、权限不足、参数非法、依赖服务报错）：未带 / 带非法 Bearer token → `401 请先登录`，前端清 token 跳回登录框；`keyword`/`status` 参数非法值 → 后端按「不过滤」兜底（status 只认白名单 `no_definition`/`has_definition`，其它当空处理）；LIKE 元字符 `%`/`_` 由 `escapeLikePattern` 转义，不产生通配注入。
- [x] 历史数据 / 存储结构变更：无表结构变更；`sessions` 表里已有 token 继续有效，老用户只需重新登录（旧 cookie 不再发送）；`words.senses` 既有 JSON 结构天然兼容 `JSON_SEARCH`，无迁移。
- [x] 原有 mock 数据 / 测试替身 / 本地持久化状态：`auth_test.go` 里基于 cookie 的用例（`TestRequireAuthNoCookie` 等）需改为基于 `Authorization` 头；`main_test.go` 需补 `handleListWords` 的 keyword/status 过滤用例；前端 token 从 localStorage 读写，属新增本地状态，无旧状态迁移。
- [x] 是否新增或变更接口（契约变更）：是——登录响应从 `user` 变为 `{ token, user }`；`GET /api/words` 新增 `keyword`/`status` 参数；新增 `POST /api/words/reset-counts`。前端与后端需同批联调，不能只改一侧。
- [x] 权限 / 角色 / 客户 / 租户差异：搜索与重置次数都是 `requireAuth` + 按 `user_id` 限定，普通用户只能操作自己的词；管理端词库搜索/导出仍走 `requireAdmin`，不受影响。
- [x] 并发、重复请求、幂等性：重置次数天然幂等（反复置 1 结果不变）；快速搜索用 300ms 防抖 + `usePaginatedList` 的 `reqSeq` 丢弃过期响应，避免竞态；删除二次确认挡住误点，后端删除按 id + user_id 精确匹配，重复提交第二次返回 404，无害。

## 验收标准

- [ ] 前端列表每页只请求/展示 20 条，滚动到底继续加载下一批 20 条。
- [ ] 直接 `curl` 任何 `/api/words*`、`/api/admin/*` 接口（不带 Authorization 头）返回 401；带合法 `Authorization: Bearer <token>` 返回数据。
- [ ] 登录接口返回 `{ token, user }`，前端刷新页面后仍保持登录（token 持久化），登出后 token 被清除。
- [ ] 首页搜索框：输入英文子串能模糊匹配单词；输入中文子串能匹配释义；选「暂无释义」能只列出 `senses` 为空的词；三者可叠加排序。
- [ ] 个人中心「重置次数」按钮：点击弹二次确认，确认后该用户所有单词 `review_count` 变为 1，`due_at`/`interval_days`/`ease_factor` 保持不变。
- [ ] 用户删词、管理删词库单条/批量，均弹二次确认，取消不删除、确认才删除。
- [ ] 管理端「导出 CSV」在 Bearer 鉴权下仍能正常下载。
- [ ] `go test ./...` 全绿（auth 用例已切到 Bearer 语义），前端 `npm run lint` 无报错。

## 未决问题

- 重置次数是否**包含已归档的单词**：当前按「含已归档」实现（需求原文「所有单词」）。若只想重置活跃词，改为 `AND archived = 0` 即可。验收前请浩原确认。（其余设计项已通过澄清确认，无其它未决。）

## 实现后追加变更（2026-08-26 实施阶段）

需求在实现后由浩原调整，以最终代码为准：

1. **用户侧快速搜索 / 「暂无释义」筛选最终移除**：改由「管理侧删除级联 + 启动迁移清理」替代，用户列表不再有搜索/筛选入口（`/api/words` 的 `keyword`/`status` 参数保留但前端不再传）。
2. **管理侧删除级联**：管理员删除词库单词（单条/批量）时，同步删除所有用户已保存的同名单词（`wordRepo.DeleteByWordKey/DeleteByWordKeys`）。
3. **启动迁移清理无释义单词**：`db.go` 的 `deleteWordsWithoutDefinition()` 在启动时删除用户侧 `senses` 为空或为「查询失败」占位、且 `translating=0` 的单词（幂等）。
4. **重置次数按钮位置**：从独立「数据操作」卡片移到「修改密码」右侧（`ChangePasswordDialog.vue`）。

「未决问题」中的「是否含已归档」最终按「含已归档」实现（`WHERE user_id=?` 不限定 archived）。

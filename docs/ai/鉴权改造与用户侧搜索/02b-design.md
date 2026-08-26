<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-26 08:00 / 结束 2026-08-26 08:12 / 本阶段 token（in/out）留空（无 /cost 命令）/ 本阶段成本 留空（无 /cost）/ 规范版本 team-dev-skills@1.0.0 -->

# 详细设计

## 接口契约 ⚠️ 前后端共享，改动需双方确认后再改

### 接口清单

| 方法 | 路径 | 用途 | 谁提供 |
|---|---|---|---|
| POST | `/api/login` | 登录，返回 token + 用户信息 | 后端 |
| GET | `/api/words` | 用户单词分页列表（新增 keyword/status 过滤） | 后端 |
| POST | `/api/words/reset-counts` | 把当前用户所有单词 review_count 重置为 1（新增） | 后端 |
| GET | `/api/admin/dictionary/export` | 导出 CSV（改为 Bearer 鉴权后前端改用 fetch 下载） | 后端 |

所有 `/api/*`（除 `/api/login`、`/api/logout`）请求统一携带请求头 `Authorization: Bearer <token>`。

### 请求 / 响应示例

#### POST /api/login

请求：

```json
{ "username": "alice", "password": "secret123" }
```

成功响应（200）：

```json
{
  "token": "6f0a1c...64 位 hex...",
  "user": {
    "id": 1,
    "username": "alice",
    "is_admin": false,
    "created_at": "2026-08-26T08:00:00+08:00",
    "last_login_at": "2026-08-26T08:00:00+08:00"
  }
}
```

字段说明：

| 字段 | 含义 | 空值时的行为 |
|---|---|---|
| `token` | 会话 token（64 位 hex 随机串，服务端存 `sessions` 表，滑动过期 30 天） | 登录成功必有，非空 |
| `user.last_login_at` | 本次登录时间 | 用户从未登录过时为 `null`（登录成功后会写入，故实际非空） |

#### GET /api/words

请求（Query 参数）：

```
/api/words?archived=0&sort=count&keyword=apple&status=no_definition&page=1&limit=20
```

成功响应（200）：

```json
{
  "items": [
    {
      "id": 12,
      "word_key": "apple",
      "display_word": "apple",
      "senses": [],
      "translating": false,
      "archived": false,
      "review_count": 3,
      "first_added_at": "2026-08-20T09:00:00+08:00",
      "last_reviewed_at": "2026-08-25T09:00:00+08:00",
      "due_at": null,
      "interval_days": 0,
      "ease_factor": 2.5
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 20,
  "has_more": false
}
```

字段说明：

| 字段 | 含义 | 空值时的行为 |
|---|---|---|
| `keyword` | 模糊搜索关键字，同时匹配 `word_key` 与 `senses[].translation` | 空或缺失 = 不过滤 |
| `status` | `no_definition`（senses 空）/ `has_definition`（senses 非空）/ 空（不过滤） | 空或非法值 = 不过滤 |
| `has_more` | 是否还有下一页（`page*limit < total`） | 后端算好，前端不用自己推 |

#### POST /api/words/reset-counts

请求：无 body。

成功响应：`204 No Content`（无响应体）。

### 错误码

| 错误码 | 含义 | 调用方应该怎么表现 |
|---|---|---|
| 400 | 请求格式不正确 / 参数非法 | 提示对应 `error` 文案 |
| 401 | 未登录 / token 无效或过期 | 前端清 token、清 currentUser、跳回首页登录框 |
| 403 | 无权限（非管理员访问 admin 接口） | 跳回首页 |
| 404 | 资源不存在 | 提示 `error` 文案 |
| 429 | 登录/改密尝试次数过多 | 提示稍后再试 |
| 500 | 服务器内部错误 | 提示 `error` 文案 |

统一错误响应体：`{ "error": "具体文案" }`。

### 边界约定

- 超时时间：普通请求 `defaultRequestTimeout`，导出 CSV `exportRequestTimeout`（后端已有，不变）。
- 分页上限 / 单次请求最大条数：`limit` 上限 `maxPageLimit=200`，前端固定请求 `limit=20`。
- 空列表 vs `null`：分页接口 `items` 恒为 `[]`（不为 `null`）；`senses` 为 `[]` 表示暂无释义。
- 时间格式与时区：`time.Time` 的 RFC3339 字符串（含时区），前端用 `formatTime` 本地化展示。
- 字段缺失时：`due_at` 为 `null`（从未闪卡复习）；`total_all` 仅词库管理接口返回，`omitempty` 缺失时不返回该 key。

### 契约变更记录

| 日期 | 改了什么 | 谁提出 | 对方是否已知 |
|---|---|---|---|
| （暂无） | | | |

## 后端详细设计

### 数据库变更

无表结构变更。`sessions`、`words`、`word_dictionary` 三张表结构不动；Bearer 只改变 token 的传输方式（cookie → 请求头）。

### 核心流程

**登录（handleLogin）**：

1. 解析 body，走登录限流校验。
2. `bcrypt` 校验密码，失败记一次失败并返回 401。
3. 生成 `randomToken()`，`sessions.Create` 落库，`users.RecordLogin` 记最后登录时间。
4. 返回 `{ token, user }`，**不再 `SetCookie`**。

**鉴权（requireAuth）**：

1. 从 `Authorization` 头提取 token（`bearerToken(r)`），无头或格式错 → 401。
2. `sessions.FindWithUser(token)` 查会话 + 用户；`sql.ErrNoRows` 或已过期 → 401。
3. 滑动续期 `sessions.Touch`，用户塞进 context，进入 next。

**登出（handleLogout）**：

1. `bearerToken(r)` 取 token，`sessions.DeleteByToken`。
2. 返回 204（无 cookie 可清）。

**改密（handleChangePassword）**：保留现逻辑，仅「当前会话 token」的获取从 `r.Cookie` 改为 `bearerToken(r)`。

**搜索（handleListWords）**：

1. 读 `keyword`（`strings.ToLower(strings.TrimSpace(...))`）、`status`。
2. `words.CountByUser(ctx, userID, archived, keyword, status)` + `words.ListPage(ctx, userID, archived, keyword, status, sort, limit, offset)`。
3. 过滤 WHERE 复用 `senseFilterWhere`（原 `dictFilterWhere` 改名）：`word_key LIKE ? OR JSON_SEARCH(senses,'one',?,'$.translation') IS NOT NULL` + status 条件，叠加 `user_id = ? AND archived = ?`。

**重置次数（handleResetReviewCounts）**：

1. `currentUser(r)` 取用户。
2. `words.ResetReviewCounts(ctx, user.ID)`：`UPDATE words SET review_count = 1 WHERE user_id = ?`。
3. 返回 204（幂等，0 行也返回 204）。

### 单元测试

> 本次改动落在「权限 / 校验 / 数据转换」档（建议测试），纯函数部分成本低、回报高，列下面几项作为改动清单显式项。

| 被测函数 / 类 | 覆盖点 | 是否已存在 |
|---|---|---|
| `bearerToken` | 无 Authorization 头→空串；`Bearer xxx`→`xxx`；`bearer xxx`（小写）→空串；空 token→空串 | 否（新增） |
| `senseFilterWhere`（原 `dictFilterWhere`） | keyword 空→不过滤；keyword 命中 word_key；status=`no_definition`/`has_definition`/空；组合场景 | 否（新增） |
| `handleLogin` / `requireAuth` / `requireAdmin` / `handleChangePassword` | 从 cookie 语义改为 Bearer 语义的 7 个用例改造 | 已存在（改） |

## 前端详细设计

### 页面 / 组件拆解

| 组件 | props | 回调事件 | 说明 |
|---|---|---|---|
| `HomeView.vue` | — | — | 新增搜索框 + 「暂无释义」筛选，`/api/words` 请求带 `keyword`/`status` |
| `ProfileView.vue` | — | — | 新增「重置次数」按钮，二次确认后调 `POST /api/words/reset-counts` |
| `AdminDictionary.vue` | — | — | 「导出 CSV」由 `<a href>` 改为 `fetch` + Blob 下载 |

### 状态管理

- **token**：存 `localStorage`（key `vocab_token`），`auth` store 的 `login` 写入、`logout` 清除、`checkAuth` 读取判断是否发起 `/api/me`。
- **请求封装**：`api/client.js` 的 `request()` 每次从 localStorage 读 token 拼 `Authorization: Bearer <token>` 头；401（非 `/api/me`）时清 token + currentUser + 跳首页。
- **搜索状态**：`HomeView` 局部 `ref`（`keyword`、`statusFilter`），变化后 300ms 防抖调 `reset()`（复用 `usePaginatedList` 的 `reqSeq` 丢弃过期响应）。

### TS / JS 类型定义

项目为 JS（非 TS），登录响应结构如下（后端 `loginResponse` 的对应映射）：

```js
// POST /api/login 成功响应
// { token: string, user: { id: number, username: string, is_admin: boolean, created_at: string, last_login_at: string | null } }
```

`Word` 的字段与后端 `Word` 结构一一对应（`word_key` / `display_word` / `senses` / `translating` / `archived` / `review_count` / `first_added_at` / `last_reviewed_at` / `due_at` / `interval_days` / `ease_factor`），无需新类型。

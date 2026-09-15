<!-- 适用任务类型：feature；涉及接口变更和前端组件状态取舍。 -->
<!-- 阶段记录：模型 GPT-5 / 开始 2026-09-15 / 结束 2026-09-15 / 规范版本 @sw/dev-skills@1.1.2 -->

# 背诵颜色配置与模糊复习详细设计

## 接口契约 ⚠️ 前后端共享，改动需双方确认后再改

### 接口清单

| 方法 | 路径 | 用途 | 谁提供 |
|---|---|---|---|
| GET | `/api/review-colors` | 登录用户读取当前次数渐变配置 | 后端 |
| GET | `/api/admin/settings` | 管理员读取设置，响应新增渐变字段 | 后端 |
| PUT | `/api/admin/settings` | 管理员保存设置，请求与响应新增渐变字段 | 后端 |
| POST | `/api/flashcards/review` | 评分字段新增 `fuzzy` 取值 | 后端 |

### 请求 / 响应示例

#### `GET /api/review-colors`

请求：无请求体；要求已登录。

成功响应：

```json
{
  "start_color": "#e4f7e9",
  "end_color": "#a11d1d",
  "segments": 6
}
```

#### `PUT /api/admin/settings`

请求：沿用现有完整设置负载，并增加以下字段。

```json
{
  "enabled": true,
  "api_key": "****",
  "base_url": "https://api.deepseek.com",
  "fallback_model": "deepseek-v4-flash",
  "thinking_model": "",
  "tts_api_key": "****",
  "tts_cluster": "volcano_tts",
  "tts_voice_type": "BV001",
  "start_color": "#e4f7e9",
  "end_color": "#a11d1d",
  "segments": 6
}
```

成功响应：与 `GET /api/admin/settings` 一致；密钥仍使用现有掩码规则。

字段说明：

| 字段 | 含义 | 空值时的行为 |
|---|---|---|
| `start_color` | 次数为第一档时的 `#RRGGBB` 背景色 | 非法或缺失时请求返回 400，不覆盖旧值 |
| `end_color` | 最高档及超出最高档次数的 `#RRGGBB` 背景色 | 非法或缺失时请求返回 400，不覆盖旧值 |
| `segments` | 渐变档位数，整数范围 2–12 | 非整数或超出范围时请求返回 400，不覆盖旧值 |

#### `POST /api/flashcards/review`

请求：现有负载的 `rating` 新增 `fuzzy`。

```json
{
  "id": 42,
  "rating": "fuzzy"
}
```

成功响应：保持现有 `Word` 响应；`review_count` 加 1，`archived` 为 `false`，并返回新的 `interval_days`、`ease_factor` 与 `due_at`。

### 错误码

| 错误码 | 含义 | 调用方应该怎么表现 |
|---|---|---|
| 400 | 颜色不是 `#RRGGBB` 或分段数不在 2–12 | 保留表单值并展示服务端错误，不显示保存成功 |
| 400 | 闪卡评分不是四个合法值之一 | 保持当前卡片，不推进队列 |
| 401 | 未登录读取颜色配置或操作闪卡 | 沿用现有鉴权跳转 |
| 403 | 非管理员修改设置 | 沿用现有无权限处理 |

### 边界约定

- `GET /api/review-colors` 使用现有 10 秒请求超时和登录鉴权；无分页。
- 颜色字段始终返回完整的 `#RRGGBB`；后端配置缺失时启动播种默认值，接口不返回 `null`。
- `segments` 始终返回整数；任何次数 `count >= 1` 的档位为 `min(floor((count - 1) / 2) + 1, segments)`，零和非法次数不着色。
- 色值为两端 RGB 线性插值：第 `i` 档比例为 `(i - 1) / (segments - 1)`；文字颜色由前端根据背景亮度自动选择黑或白，保证可读性。
- 例句中命中词按其 `occurrence_count` 使用上述档位和色值；近义词、反义词、形近词命中时只使用 `var(--accent)`，不使用渐变。

### 契约变更记录

| 日期 | 改了什么 | 谁提出 | 对方是否已知 |
|---|---|---|---|
| 2026-09-15 | 初始契约：新增渐变读取接口、设置字段和 `fuzzy` 评分 | Alan-codex | 是 |

## 后端详细设计

### 数据库变更

不新增表或列。`settings` 表新增三行键值：`review_color_start`、`review_color_end`、`review_color_segments`；由 `loadSettings` 幂等播种。

### 核心流程

1. 服务启动时，`loadSettings` 为三项颜色设置播种默认值，`refreshSettingsCache` 读入 `App` 受读写锁保护的颜色配置。
2. 管理员读取或提交设置时，`settingsView` 和 `updateSettingsRequest` 携带三项字段。更新处理器先校验两种颜色与分段范围，验证成功才与现有设置同批 `UpsertMany`，随后刷新缓存。
3. 登录用户请求 `GET /api/review-colors` 时，处理器从缓存返回只含三项非敏感数据的颜色配置。
4. 闪卡提交 `fuzzy` 时，`validFlashcardRating` 接受该值；`applySRSScheduling` 在首次复习返回 1 天，否则将既有间隔按 1.1 倍向上取整，并使难度系数减少 0.10。处理器不归档该词，其他写回逻辑不变。

### 单元测试

按项目根 `AGENTS.md` 约束，本次修改后只执行编译验证，不执行自动化测试。本次不新增或修改测试文件；既有 `TestApplySRSScheduling` 保持原有三档断言。实现中的颜色解析与排期分支保持小函数和显式常量，便于后续补测。

## 前端详细设计

### 页面 / 组件拆解

| 组件 | props | 回调事件 | 说明 |
|---|---|---|---|
| `AdminSettings.vue` | 无 | 保存现有设置 | 在现有设置表单中增加起始色、终止色色彩输入与分段数输入，沿用现有加载、提交和提示状态 |
| `useReviewColors.js` | 无 | `ensure()` | 共享登录用户颜色配置，首次使用时读取 `/api/review-colors`，失败时保留默认配置 |
| `reviewLevel.js` | 颜色配置与次数 | 无 | 输出档位、RGB 插值色和可读文字色，供徽标与例句共用 |
| `WordCard.vue` / `FlashcardView.vue` / `WordLookupTooltip.vue` | 既有词条数据 | 既有查词与评分事件 | 为次数徽标和全部英文例句传入共享颜色；为词组引用根据词库命中追加主题色 |
| `AdminDictionary.vue` | 既有词库条目 | 无 | 为管理员词库表中的出现次数徽标使用共享颜色 |

### 状态管理

- `useReviewColors` 模块级 `ref` 保存 `{ start_color, end_color, segments }`，初值为详细设计中的默认值；`ensure()` 使用一次在途 Promise 去重。
- 管理员保存成功只更新本地表单。其它已打开用户页面不建立推送同步；下一次页面加载会读取新配置，符合现有设置页面的刷新语义。
- `useVocabularyIndex` 保持全局词库索引职责不变。`highlight.js` 从索引获取命中次数，不再只高亮当前词；`WordCard.vue`、`FlashcardView.vue` 与 `WordLookupTooltip.vue` 均调用它渲染英文例句；引用词从同一索引仅判断是否命中。

### TS 类型定义

项目使用 JavaScript，无 TypeScript 类型文件。前端以如下对象形状约束数据：

```js
const reviewColors = {
  start_color: '#e4f7e9',
  end_color: '#a11d1d',
  segments: 6,
}
```

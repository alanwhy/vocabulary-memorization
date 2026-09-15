<!-- 适用任务类型：feature。逐项勾选，边做边勾。 -->
<!-- 阶段记录：模型 GPT-5 / 开始 2026-09-15 / 结束 2026-09-15 / 规范版本 @sw/dev-skills@1.1.2 -->

# 背诵颜色配置与模糊复习实现计划

## 背景

- **任务类型**：feature
- **一句话做什么**：让管理员配置次数渐变色与分段，并将其统一应用于次数和例句命中词，同时为闪卡增加“模糊”排期与词组主题色命中提示。

## 改动方案

### 改动清单

- [x] `backend/app.go` —— 为 `App` 增加受现有 `settingsMu` 保护的颜色配置缓存及读取方法；不改动设置仓储接口 —— 后端编译通过。（依赖第 2 项）
- [x] `backend/settings.go` —— 定义三项配置键、默认值、`reviewColorConfig`、格式校验，并扩展播种、缓存刷新、管理员设置请求/响应/保存逻辑 —— 后端编译通过。（依赖第 1 项）
- [x] `backend/main.go` —— 注册 `GET /api/review-colors`（登录鉴权）并实现只返回颜色配置的处理器；允许闪卡请求的 `rating=fuzzy` —— 后端编译通过。（依赖第 2 项）
- [x] `backend/models.go` —— 在 `applySRSScheduling` 中加入 `fuzzy` 的 1.1 倍间隔和 -0.10 难度规则，保持既有三档不变 —— 后端编译通过。（可与第 2 项并行）
- [x] `frontend/src/composables/useReviewColors.js` —— 新增共享颜色配置状态、默认回退值和请求去重的 `ensure()` —— 前端构建通过。（依赖第 3 项）
- [x] `frontend/src/utils/reviewLevel.js` —— 用 `02b-design.md` 的档位公式和 RGB 插值替代六档固定映射，导出次数徽标与例句共用的动态样式计算 —— 前端构建通过。
- [x] `frontend/src/utils/highlight.js` —— 例句分词恢复使用词库索引中每个命中词的出现次数；引用词保留可点击拆分并暴露词库命中状态 —— 前端构建通过。（依赖第 6 项）
- [x] `frontend/src/assets/main.css` —— 删除写死的次数/例句六档热力变量和级别样式，保留结构类、悬停与主题色命中类 —— 前端构建通过。（依赖第 6 项）
- [x] `frontend/src/components/admin/AdminSettings.vue` —— 在现有设置表单、响应绑定和提交负载中加入两种颜色和分段数输入 —— 前端构建通过。（依赖第 2 项）
- [x] `frontend/src/components/admin/AdminDictionary.vue` —— 接入共享颜色配置并将词库出现次数徽标改为动态样式 —— 前端构建通过。（依赖第 5、6 项）
- [x] `frontend/src/components/word/WordCard.vue` —— 接入共享颜色与词库索引：例句按命中次数使用渐变，近反义/形近词命中时使用主题色 —— 前端构建通过。（依赖第 5、6、7、8 项）
- [x] `frontend/src/views/FlashcardView.vue` —— 增加“模糊 🤔”评分按钮；以同一共享颜色渲染次数、例句和引用词主题色标记 —— 前端构建通过。（依赖第 4、5、6、7、8 项）
- [x] `frontend/src/components/word/WordLookupTooltip.vue` —— 接入共享颜色与词库索引，将弹层英文例句分词后按命中次数使用同一动态渐变 —— 前端构建通过。（依赖第 5、6、7、8 项）

### 本次不做

- 不提供配置实时推送、跨已打开页面即时换色或配置历史版本；用户重新加载页面后读取新值。
- 不改变词库的出现次数统计、个人背诵次数统计或历史闪卡排期数据。
- 不新增“模糊”次数的独立统计报表，也不调整现有 `good`、`hard`、`again` 按钮的语义。
- 不把近义词、反义词、形近词按出现次数做渐变；它们只表示系统是否存在该词。

## 关键文件

| 文件 | 改动性质（新增/修改/删除） | 具体改哪个函数/类型/模式 | 谁调用它 / 它影响谁（callers/callees 摘要） |
|---|---|---|---|
| `backend/app.go` | 修改 | `App` 的设置缓存字段与颜色配置读取方法 | `settings.go` 刷新缓存，`main.go` 颜色读取接口使用 |
| `backend/settings.go` | 修改 | `loadSettings`、`refreshSettingsCache`、`settingsView`、`updateSettingsRequest`、`handleUpdateSettings` | `handleUpdateSettings` 调用 `UpsertMany`、刷新缓存和 JSON 写入；其调用者是管理员路由 |
| `backend/main.go` | 修改 | 路由注册、`validFlashcardRating`、`handleReviewColors` | `handleFlashcardReview` 是 `applySRSScheduling` 的业务调用方 |
| `backend/models.go` | 修改 | `applySRSScheduling` | CodeGraph callers：`handleFlashcardReview`、`TestApplySRSScheduling`；无 callees |
| `frontend/src/composables/useReviewColors.js` | 新增 | 颜色配置单例与 `ensure()` | `AdminDictionary.vue`、`WordCard.vue`、`FlashcardView.vue` 消费 |
| `frontend/src/utils/reviewLevel.js` | 修改 | 档位、色值插值、可读文字色计算 | 次数徽标和例句高亮的统一来源 |
| `frontend/src/utils/highlight.js` | 修改 | `tokenizeExample`、`splitWordRef` 的命中数据 | CodeGraph callers：`WordCard.vue`、`FlashcardView.vue` |
| `frontend/src/assets/main.css` | 修改 | 徽标、例句高亮、主题色命中基础类 | 被所有单词展示组件的 class 使用 |
| `frontend/src/components/admin/AdminSettings.vue` | 修改 | `form`、设置加载/保存与表单项 | 调用现有管理员设置 GET/PUT |
| `frontend/src/components/admin/AdminDictionary.vue` | 修改 | 出现次数徽标的动态样式 | 使用词库条目的 `occurrence_count` |
| `frontend/src/components/word/WordCard.vue` | 修改 | 例句 tokens、引用词渲染和颜色状态 | CodeGraph callers：`tokenizeExample` 与 `splitWordRef` 的页面入口之一 |
| `frontend/src/views/FlashcardView.vue` | 修改 | 评分数组、例句 tokens、引用词与次数渲染 | CodeGraph callers：`tokenizeExample`、`splitWordRef` 的页面入口之一 |
| `frontend/src/components/word/WordLookupTooltip.vue` | 修改 | 英文例句分词与动态颜色样式 | 由单词点击查询流程调用，和其它例句入口共享 `tokenizeExample` |

## 验证方式

### 验证前置 ⚠️ 必填，写代码之前填

项目根 `AGENTS.md` 明确要求：代码修改后只做编译验证，不执行单元测试、接口验证或浏览器验收。因此本任务的唯一验证为双端编译。

- **命令 + 预期输出**：

  ```text
  命令：cd backend && go build ./...
  实际：退出码 0，无编译错误。

  命令：cd frontend && npm run build
  实际：Vite 8.2.0 成功构建 2192 个模块；仅报告既有的产物体积警告，无编译错误。
  ```

### 受影响的测试

`codegraph affected` 对 `backend/models.go`、`backend/settings.go`、`frontend/src/utils/reviewLevel.js`、`frontend/src/utils/highlight.js`、`frontend/src/views/FlashcardView.vue` 均未报告受影响测试文件。

- [x] 按项目验证约束，不新增、修改或执行自动化测试；仅执行上述编译验证。

## 影响面与风险

### 风险点

- 配置接口与管理员设置的字段名不一致会使保存后的页面回退到默认颜色，实施时以 `02b-design.md` 的字段名为唯一契约。
- RGB 插值产生的中间色可能与深色主题对比不足，必须根据背景亮度自动选择黑/白文字色。
- 词库索引加载是异步的；加载前不得误标不存在，只能保持普通文本，索引返回后再响应式更新。
- 弹层例句目前是整句文本，实施时要保留其只读语义，不新增点击查词等无关交互。

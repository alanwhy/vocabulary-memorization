<!-- 适用任务类型：feature，涉及接口字段变更 + 前端渲染取舍。 -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-25 / 结束 2026-08-25 09:12 / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 详细设计

## 接口契约 ⚠️ 前后端共享

### 接口清单

无新增路由、无新增 HTTP 接口。本次是**存量数据结构 `Sense` 的字段增量**，影响以下接口的响应体（均为纯增量，omitempty，向后兼容）：

| 方法 | 路径 | 变化 |
|---|---|---|
| POST | `/api/words` | 返回的 `Word.senses[].*` 新增 6 个可选字段 |
| GET | `/api/words` | 同上 |
| GET | `/api/words/translating` | 同上 |
| GET | `/api/flashcards/queue` | 同上 |
| GET | `/api/admin/dictionary` | `dictionaryEntry.senses[].*` 新增 6 个可选字段 |
| POST | `/api/admin/dictionary/retry` | 返回的 `senses` 同上 |
| GET | `/api/vocabulary` | 新增：全局词库索引 `[{word_key, occurrence_count}]`，供前端高亮判断 |

### 请求 / 响应示例

#### `Sense` 结构（响应内嵌对象）

强化前的存量数据（旧格式，仍合法）：

```json
{ "pos": "n.", "translation": "苹果；苹果树；苹果公司" }
```

强化后新查词返回（新格式，词级字段在每条上重复）：

```json
[
  {
    "pos": "n.",
    "translation": "苹果；苹果树；苹果公司",
    "phonetic": "/ˈæp(ə)l/",
    "example": "She bit into a crisp red apple.",
    "root": "",
    "affix": "",
    "synonyms": [],
    "antonyms": []
  }
]
```

词根词缀非空示例（`serendipity`）：

```json
[
  {
    "pos": "n.",
    "translation": "机缘巧合；意外发现珍奇事物的能力",
    "phonetic": "/ˌserənˈdɪpəti/",
    "example": "Finding that old letter was pure serendipity.",
    "root": "serendip",
    "affix": "-ity",
    "synonyms": ["chance", "fortuity"],
    "antonyms": []
  }
]
```

字段说明（只写从名字看不出含义的字段）：

| 字段 | 含义 | 空值时的行为 |
|---|---|---|
| `phonetic` | 国际音标，词级 | 空字符串 `""`；前端不渲染 |
| `example` | 英文例句，词义级（每条词性对应一句） | 空字符串；前端不渲染 |
| `example_translation` | 例句的中文翻译 | 空字符串；前端不渲染 |
| `root` | 词根 | 空字符串（无词根时） |
| `affix` | 词缀 | 空字符串（无词缀时） |
| `synonyms` | 近义词数组，每个元素 `英文词（中文释义）` | 空数组 `[]` |
| `antonyms` | 反义词数组，每个元素 `英文词（中文释义）` | 空数组 `[]` |
| `lookalikes` | 形近词数组，每个元素 `英文词（中文释义）` | 空数组 `[]` |

### 错误码

无新增错误码。查词补全复用现有 `failedSenses` 占位与重试逻辑。

### 边界约定

- **字段缺失 / 空值**：`omitempty` 序列化，空字段直接不返回 key（`phonetic`/`example`/`root`/`affix` 为空串、`synonyms`/`antonyms` 为空数组时都不出现）。前端统一用 `s.phonetic || ''` 防御，不假设字段一定存在。
- **词级字段的权威来源**：`phonetic`/`root`/`affix`/`synonyms`/`antonyms` 是词级信息，但平铺在每条 `Sense` 上重复出现；前端只取 `senses[0]` 展示词级信息，`example` 逐条展示。
- **向后兼容**：旧数据只含 `pos`/`translation`，前端逻辑（`v-if="s.pos"` 等）不变，新字段用 `v-if="s.phonetic"` 这类条件渲染，缺失不报错。

## 后端详细设计

### 数据库变更

无表结构变更、无迁移。`words.senses` / `word_dictionary.senses` 的 JSON 数组元素从 `{pos, translation}` 变为 `{pos, translation, +6 可选字段}`，旧数据天然兼容。

**唯一的数据库访问行为变更**在 `dictionaryRepo.SaveSenses`（`backend/store.go`）：写入条件从「仅当缓存为空」改为「仅当缓存为空或未强化」，用于历史词条升级覆盖：

```sql
UPDATE word_dictionary SET senses = ?
WHERE word_key = ?
  AND (senses IS NULL
       OR JSON_LENGTH(senses) = 0
       OR JSON_EXTRACT(senses, '$[0].phonetic') IS NULL
       OR JSON_EXTRACT(senses, '$[0].phonetic') = '')
```

语义：只有「当前缓存里还没有强化数据」时才写入，避免并发下用旧数据覆盖已强化的好数据，也避免重复请求把同一条覆盖来覆盖去。

### 核心流程

补全复用现有查词管线，只改触发判定与写回语义：

```
1. handleAddWord（新增录入）
   - lookupDictionarySenses → cacheHit
   - needsEnrichment = cacheHit && !sensesEnriched(cachedSenses)
   - translating = !cacheHit || needsEnrichment
   - Insert(..., translating, ...)
   - 若 translating → spawnTranslation(wordID, wordKey)
   - spawnTranslation → translateAndSave：
       a. 调 translateWord（新 prompt，返回带 6 字段的 Sense）
       b. mergeSensesByPos（保留新字段）
       c. saveWordSenses（UpdateSenses，写用户 words + translating=0）
       d. saveDictionarySenses（SaveSenses 升级覆盖缓存）

2. tryIncrementExisting（再次录入同一词）
   - FindByUserAndKey → existing（含 senses + translating）
   - 若 !sensesEnriched(existing.Senses) && !existing.Translating：
       a. MarkTranslating（新增：UPDATE words SET translating=1, translation_started_at=?）
       b. existing.Translating = true（响应里让前端显示「查词中」并轮询）
       c. spawnTranslation(existing.ID, existing.WordKey)
   - 其余照旧（次数 +1、刷新时间、返回 existing）
```

判定函数（新增纯函数，`backend/models.go`）：

```go
// sensesEnriched 判断一组释义是否已包含强化字段。
// 以第一条的 phonetic（音标）为非空作为「已强化」的可靠信号：音标是每个英文词必有、
// 而旧 prompt 一定没返回的字段；root/affix/synonyms/antonyms 对某些词天然为空，不能作判据。
func sensesEnriched(senses []Sense) bool {
    if len(senses) == 0 {
        return false
    }
    return strings.TrimSpace(senses[0].Phonetic) != ""
}
```

### 单元测试

本次改动落在「数据转换 / 业务规则」层，按验证要求分层表属「建议测试」档，新增/扩展纯函数单测：

| 被测函数 / 类 | 覆盖点 | 是否已存在测试 |
|---|---|---|
| `sensesEnriched`（新增） | 空数组→false；非空但无 phonetic→false；有 phonetic→true；phonetic 为空白串→false | 否，新增 |
| `mergeSensesByPos` | 合并同词性时保留词级字段（取组内第一个非空）与 example；不因新字段破坏既有 pos/translation 合并 | 部分（3 个旧用例），新增 1 个 |
| `parseSenses` | 解析 6 个新字段；容忍 markdown 包裹；空 synonyms/antonyms 数组不报错 | 否，新增 |

## 前端详细设计

### 页面 / 组件拆解

三处渲染 `Sense` 的组件统一扩展，词级字段只从 `senses[0]` 取：

| 组件 | 改动 | 展示内容 |
|---|---|---|
| `frontend/src/components/word/WordCard.vue` | 单词下方加一行「音标」，每个 sense 下加例句，词级行加词根词缀/近反义 | `senses[0].phonetic`、每条的 `s.example`、`senses[0].root/affix`、`senses[0].synonyms/antonyms` |
| `frontend/src/views/FlashcardView.vue` | 翻面后同 WordCard 一致地展示强化字段 | 同上 |
| `frontend/src/components/admin/AdminDictionary.vue` | 词库「释义」列展示音标（其余强化字段不展开，避免表格过宽） | `d.senses[0]?.phonetic` |

### 状态管理

无新增全局状态。强化字段随 `Word.senses` / `dictionaryEntry.senses` 一起由现有轮询（`useTranslatingPoll`）和列表加载返回，无需新 store。

### TS 类型定义

本项目前端为 JS（`jsconfig.json`，无 `.ts`），无显式 TS interface 需要维护。组件内直接访问 `s.phonetic`、`s.example` 等字段，统一用 `v-if` 条件渲染防御空值。

## 契约变更记录

| 日期 | 改了什么 | 谁提出 | 对方是否已知 |
|---|---|---|---|
| 2026-08-25 | `Sense` 新增 `example_translation`（例句中文翻译）；`synonyms`/`antonyms` 元素格式改为 `英文词（中文释义）` | 用户反馈「例句/近义词/反义词需中文释义」 | 已同步（前后端同一仓库一并改） |
| 2026-08-25 | `Sense` 新增 `lookalikes`（形近词）；新增接口 `GET /api/vocabulary`；前端对例句/近反义/形近词中命中全局词库的词按出现次数分级高亮（复用 countLevel 六档配色） | 用户反馈「增加形近词查询 + 词库词高亮」 | 已同步（前后端同一仓库一并改） |

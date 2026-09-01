<!-- 适用任务类型：feature，涉及接口 + 表结构 + 前端组件取舍，触发 02b-design -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-09-01 08:06 / 结束 2026-09-01 08:15 / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 详细设计

## 接口契约 ⚠️ 前后端共享

### 接口清单

| 方法 | 路径 | 用途 | 谁提供 |
|---|---|---|---|
| PUT | `/api/words/{id}/important` | 覆盖设置某单词的「重要释义」义项列表（全量替换，幂等） | 后端 |

### 请求 / 响应示例

#### `PUT /api/words/{id}/important`

请求：

```json
{ "glosses": ["跑", "忍受"] }
```

成功响应（200）：

```json
{ "id": 123, "important_glosses": ["跑", "忍受"] }
```

清空全部标记的请求：

```json
{ "glosses": [] }
```

字段说明：

| 字段 | 含义 | 空值时的行为 |
|---|---|---|
| `glosses` | 被标记为「重要」的义项文本数组，元素是 `senses[].translation` 按「；」拆分后的某个义项原文 | 传 `[]` 表示清空全部标记，合法 |
| `important_glosses`（响应） | 服务端规范化后的最终列表（已去空/去重/截断） | 始终返回数组，空时为 `[]`，不返回 `null` |

### 错误码

| 错误码 | 含义 | 调用方应该怎么表现 |
|---|---|---|
| 400 | 参数非法：`glosses` 非数组、元素含非字符串、单条超长、条数超上限 | 回滚乐观更新，提示「保存失败」 |
| 404 | 单词不存在，或不属于当前登录用户 | 提示单词不存在/无权操作 |
| 401 | 未登录 | 由 `requireAuth` 统一跳转登录，前端无需特判 |

### 边界约定

- 超时时间：沿用 `defaultRequestTimeout`（与其它 `/api/words/*` 接口一致）。
- 单次请求最大条数：`glosses` 最多 20 条，单条 ≤ 200 字符，超出截断或 400（见后端详细设计的 `normalizeGlosses`）。
- 空列表 vs `null`：`important_glosses` 约定**始终用数组**，空用 `[]`，不用 `null`。
- 字段缺失：老数据 `words` 行无该列时，读侧归一为 `[]`，不向前端暴露 `null`。
- 时间格式：本接口不涉时间。

### 契约变更记录

| 日期 | 改了什么 | 谁提出 | 对方是否已知 |
|---|---|---|---|
| — | 初始契约 | — | — |

## 后端详细设计

### 数据库变更

| 表名 | 字段 | 类型 | 长度 | 索引 / 主键 | 默认值 | 是否非空 | 新增或变更说明 |
|---|---|---|---|---|---|---|---|
| words | important_glosses | JSON | — | 无 | NULL | 否 | 新增；存被标记为重要的义项文本数组，与 `senses` 解耦 |

迁移：新增 `migrateWordsImportantGlossesColumn()`，用 `columnExists("words", "important_glosses")` 守卫后 `ALTER TABLE words ADD COLUMN important_glosses JSON NULL`，在 `migrateSchema` 末尾调用。与既有 `migrateWordsSRSColumns` 等一致。

### 核心流程

写路径（标记/清空）：

1. 前端点击义项 → 计算新的全量义项列表 → `PUT /api/words/{id}/important`。
2. `handleSetWordImportant`：`requireAuth` → 解析 body → `normalizeGlosses` 校验（trim、去空、去重、单条 ≤200、总数 ≤20）→ `a.words.FindByIDs(ctx, user.ID, []int{id})` 校验归属（空 → 404）→ `a.words.UpdateImportantGlosses(...)` 写库 → 返回 `{id, important_glosses}`。
3. 异常：body 非法 → 400；写库失败 → 500。

读路径：所有取词查询（`ListPage` / `DueFlashcards` / `FindByIDs` / `FindByUserAndKey` / `FindTranslating*`）都经 `scanWordRows` 读 `wordColumns`，追加 `important_glosses` 列后，`Word` 自动带上该字段，闪卡与列表无需各自改查询。`NULL`/空 JSON 归一为 `[]string{}`。

覆盖关系：后台查词写回只 `UPDATE words.senses`（`saveWordSenses`/`UpdateSenses`），不触碰 `important_glosses`，因此补全/重新查询不会冲掉标记。

### 单元测试

| 被测函数 / 类 | 覆盖点 | 是否已存在测试 |
|---|---|---|
| `normalizeGlosses(in []string) []string` | 空输入返回空、去空格、去空串、去重、单条超长截断或丢弃、总数超 20 截断 | 否（新增，见 `models_test.go` 或新 `normalize_test.go`） |

后端其余改动是列追加 + 直白的 CRUD 方法 + handler，属「定制页面/表单」同档的低逻辑改动，不强制加测试；`normalizeGlosses` 是纯函数校验，落「建议测试」档，补一个轻量用例。

## 前端详细设计

### 页面 / 组件拆解

| 组件 | props | 回调事件 | 说明 |
|---|---|---|---|
| `FlashcardView.vue` | —（`cards` 为本地 ref） | — | 背面 `.translation` 按义项循环渲染；点击 toggle 并本地乐观更新 + `apiPut` |
| `WordCard.vue` | `word`、`mode` | 新增 `set-important` | 受控展示组件，点击义项**不直接改 prop**，emit 新列表交给父级 |
| `WordList.vue` | `words`、`mode`、… | 新增透传 `set-important` | 把卡片事件透传给 `HomeView` |
| `HomeView.vue` | — | — | 处理 `set-important`：乐观更新 `w.important_glosses` + `apiPut`，失败回滚 + `ElMessage` |

### 状态管理

- `FlashcardView`：卡片是页面局部 `cards` ref，点击直接改 `current.important_glosses`，`apiPut` 失败回滚。
- 单词列表：`words` 由 `usePaginatedList` 在 `HomeView` 维护，`WordCard` 只读 `word` prop；点击 emit → `HomeView` 先同步改 `w.important_glosses`（即时反馈）再 `apiPut`，失败还原。
- 义项「是否重要」的判断：各组件用 `splitGlosses(s.translation)` 得到义项数组，再 `new Set(word.important_glosses || [])` 判断命中。

### 数据形状（本项目前端为 JS，无 TS）

```js
// utils/gloss.js —— 新增
export function splitGlosses(translation) {
  if (!translation) return []
  return translation.split('；').map((s) => s.trim()).filter(Boolean)
}

// Word 对象新增字段（后端返回，前端消费）
// word.important_glosses: string[]  被标记为重要的义项文本；空数组表示无标记
```

渲染示意（Flashcard 与 WordCard 共用同一结构）：

```vue
<span class="translation">
  <template v-for="(g, gi) in splitGlosses(s.translation)" :key="gi">
    <span
      :class="{ 'gloss-important': importantSet.has(g) }"
      @click.stop="toggleGloss(g)"
    >{{ g }}</span>
    <span v-if="gi < splitGlosses(s.translation).length - 1">；</span>
  </template>
</span>
```

新增样式：`.gloss-important { font-weight: 700; }`（两处组件各自的 scoped style 各加一份）。

`pending`（查词中/暂无释义）与 `error` 分支的 `.translation` 是占位单字符串，不拆义项、不加点击，保持不变。

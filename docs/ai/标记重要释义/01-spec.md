<!-- 适用任务类型：feature -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-09-01 08:02 / 结束 2026-09-01 08:15 / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 点击中文释义标记「重要释义」，重要释义加粗显示

## 需求陈述

用户在看释义时，希望点某个中文意思把它标成「重要释义」，被标记的释义**加粗**显示，突出这个词最核心、最该记的那几个意思。粒度到**义项级**（一个词性下由「；」分隔的每个中文意思各自可点、各自加粗），一个词**可标多个**，在**闪卡背面**和**单词列表（WordCard）**两处都生效。

## 定制 or 通用 ⚠️ 必填

- [x] **通用能力** —— 受益范围：本应用的所有用户（个人单词本应用，无多客户/多租户概念）

说明：与上一期「单词信息强化」一致。本仓库是单部署 Go 后端 + Vue 前端，`.claude/ai-profile.md` 尚无公共层/定制层声明（layer-detect 未识别出 `common/shared/customers/...` 命名目录），判定直接落「通用能力」，无公共层下沉风险。

## 影响面

`codegraph impact "Word"` 的输出（13 个受影响符号）：

```
backend/models.go      struct Word:121
backend/main.go        handleAddWord:158
backend/store.go       scanWordRows:209 / DueFlashcards / ListPage / FindTranslatingByUser / FindByIDs / FindByUserAndKey / FindTranslating / FindTranslatingStale
backend/app.go         wordStore 接口的 FindByUserAndKey / FindTranslating / FindTranslatingStale
```

补充说明：本次**不改 `Sense` 结构**（重要标记存独立列，见「设计要点」），`Sense` / `mergeSensesByPos` 均不动。改的是 `Word` 结构 + `words` 表新增一列 + 前端两处渲染。所有返回单词的查询都经 `scanWordRows` 读 `wordColumns`，所以给 `Word` 加字段 + 给 `wordColumns` 加列后，闪卡队列、单词列表、FindByIDs、Review 返回都会自动带上 `important_glosses`。

- **已确认事实**：上面的 codegraph 原始输出。
- **推断**：本次改动涉及后端约 6 个文件（schema.sql / db.go / models.go / store.go / app.go / main.go）+ 前端约 4 个文件（FlashcardView.vue / WordCard.vue / HomeView.vue / 新增 utils/gloss.js），合计 > 3 个文件 → 计划阶段（`/sw-plan`）必需。

## 核心语义与数据模型 ⚠️ 必填

- **关键 ID 含义** `[已确认]`：重要标记挂在 `words` 表的一行上，定位键是 `words.id + user_id`（与既有「重试 / 归档 / 删除」等接口的归属校验一致）。`word_key` 不用于重要标记（它是 `words` 与全局 `word_dictionary` 的关联键，而重要标记是**个人**语义，不能进全局词典）。

- **核心数据存哪、怎么流转** `[已确认，本次新增]`：新增 `words.important_glosses JSON` 列，存「被标记为重要的义项文本」数组，例如 `["跑", "忍受"]`。它独立于 `senses`（`senses` 仍是 LLM 生成的释义，`important_glosses` 是用户手标的重点）。流转：前端点击义项 → 计算新的全量列表 → `PUT /api/words/{id}/important` 写回该列 → 下次取词随 `scanWordRows` 一起返回。

- **义项如何识别** `[已确认]`：`senses[].translation` 是 `mergeSensesByPos` 用「；」（全角分号）合并出的字符串。前端按「；」拆分、trim 后得到义项列表；某义项「重要」当且仅当其文本精确命中 `important_glosses` 里的某项。

- **多个入口行为是否一致** `[已确认]`：闪卡队列（`DueFlashcards`）、闪卡单条（`FindByIDs`）、单词列表（`ListPage`）都走 `scanWordRows`，返回的 `important_glosses` 一致；前端闪卡与 WordCard 用同一套拆分/加粗逻辑。CSV 导出（`formatSenses`）与词云（`cloudMeaning`）只消费 `pos/translation`，不涉及个人标记，行为不变。

- **和数据相关的边界情况** `[已确认/推断]`：见「边界行为清单」。要点——后台查词写回 `senses` 时**不会**触碰 `important_glosses`，标记不丢；但若重查后 LLM 返回的义项文本变了，旧标记文本失配，会变成「存了但不显示」的孤儿标记（无害，不自动清理）。

## 设计要点

1. **独立列存储，与 LLM 释义解耦**。`important_glosses` 单独一列，不进 `senses` JSON。理由：`senses` 会在录入补全、`handleRetryWord`、`handleFlashcardReview` 的「again + 需补全」分支里被后台查词整段覆盖，若把标记嵌进 `senses`，这些路径必须逐条保留用户标记、极易漏。独立列只由用户点击动作写，LLM 管线永不触碰，标记天然存活。

2. **义项按「；」拆分（纯前端）**。不改变 `mergeSensesByPos` 的合并语义、不拆分存储结构，只在渲染层把 `translation` 按「；」拆成义项逐条渲染。新增 `utils/gloss.js` 提供 `splitGlosses(translation)`，闪卡与 WordCard 共用。

3. **API 用「全量 PUT」，不用增删 toggle**。`PUT /api/words/{id}/important`，body `{ "glosses": ["跑", "忍受"] }`，后端整体覆盖。理由：幂等、无快速连点的 toggle 竞态；前端做乐观更新，失败回滚。校验：数组元素去空格、去空串、去重、单条长度上限（如 200）、总数上限（如 20），非法直接 400。

4. **前端渲染与交互**。`.translation` 从单个 span 改成义项循环：每个义项独立 span，`@click.stop` 切换（避免闪卡翻面 / 列表其它点击被误触发），命中 `important_glosses` 的加 `gloss-important` class 加粗（`font-weight: 700`）。WordCard 是受控展示组件（`word` 为 prop，不可直接改），改为 emit 事件由父组件 `HomeView.vue` 调 API 并更新列表；FlashcardView 的卡片是本地 ref，直接更新并 PUT。

5. **被否方案记录**：
   - 「把重要标记嵌进 `senses` 的某个字段（如 `Sense.Important []string`）」—— 不做，会被后台查词覆盖，需要污染所有写回路径。
   - 「词性级标记」—— 不做，用户已确认义项级（更精准）。
   - 「单个 toggle 接口（POST 加 / DELETE 减）」—— 不做，快速连点有竞态，全量 PUT 更稳。
   - 「重要标记存全局 `word_dictionary`」—— 不做，重要是个人语义，进全局会被其它用户看到。

## 可复用资产 ⚠️

| 资产类型 | 已发现的候选 | 是否复用 | 不复用的原因 |
|---|---|---|---|
| 列迁移模式 | `migrateWordsSRSColumns` / `migrateWordsArchivedColumn`（`columnExists` + `ALTER TABLE`） | ✅ 复用 | 新增 `migrateWordsImportantGlossesColumn` |
| 路由 + 归属校验 | `handleUnarchiveWord` / `handleRetryWord`（`requireAuth` + `FindByIDs` 校验该词属于当前用户） | ✅ 复用 | — |
| store 更新方法 | `SetArchived` / `UpdateSenses`（`UPDATE ... WHERE id = ? AND user_id = ?`） | ✅ 复用 | 新增 `UpdateImportantGlosses` |
| 前端 API | `apiPut`（`frontend/src/api/client.js`） | ✅ 复用 | — |
| 义项拆分 | 无现成工具 | 新增 | `utils/gloss.js` |

## 边界行为清单 ⚠️ 必填

- [x] **空 / 无数据场景**：`important_glosses` 为空（新词 / 从未标记）→ 返回 `[]`（`scanWordRows` 里把 NULL/空 JSON 归一为 `[]string{}`），前端不报错、无义项加粗；`PUT` 传空数组 `[]` 表示清空全部标记，合法。
- [x] **异常状态**：`PUT` 参数非法（非数组、元素非字符串、超长、超量）→ 400 + 中文错误信息；单词不属于当前用户 → 404（沿用 `FindByIDs` 校验）。网络失败 → 前端回滚乐观更新、显示错误，不影响其余渲染。
- [x] **历史数据 / 存储结构变更**：新增列 `important_glosses JSON NULL`，老行该列为 NULL → `scanWordRows` 归一为空数组，无需数据回填。迁移幂等（`columnExists` 守卫），`schema.sql` 与 `migrateSchema` 同步更新，保证新装机与老库升级一致。
- [x] **原有 mock / 测试替身 / 本地持久化状态**：`wordStore` 接口新增 `UpdateImportantGlosses`，需同步给测试里的 fake 实现（若 fake 用接口断言）；尽量不给 `dictionaryStore` 加任何方法。无本地 mock 数据。
- [x] **是否新增或变更接口（契约变更）**：新增 `PUT /api/words/{id}/important`；`Word` 返回体新增 `important_glosses` 字段（纯增量）。前端 TS 类型 / 组件读取方同步。契约变更细节见 `02b-design.md`「接口契约」块。
- [x] **权限 / 角色 / 客户 / 租户差异**：标记只作用于当前登录用户自己的 `words` 行（`WHERE user_id = ?`），管理员不能标别人的词；无租户差异。全局 `word_dictionary` 不写、不受影响。
- [x] **并发、重复请求、幂等性**：全量 `PUT` 幂等，快速连点以最后一次请求为准，无 toggle 竞态；乐观更新在并发回包时序错乱时以服务端返回为准刷新。同义词性下出现相同义项文本（如 `v. 跑` 与 `n. 跑`）会同时加粗——个人应用可接受，本期不按 `pos` 区分。

## 验收标准

- [ ] `words` 表新增 `important_glosses JSON` 列，`schema.sql` 与 `migrateSchema` 均更新，老库启动后自动补齐（`columnExists` 幂等）。
- [ ] `Word` 结构含 `ImportantGlosses []string`（json `important_glosses`），`wordColumns` / `scanWordRows` 同步；`NULL` 与空值归一为 `[]`，不返回 `null`。
- [ ] `PUT /api/words/{id}/important` 可用：传 `{ "glosses": [...] }` 覆盖写入；传空数组清空；非本人单词返回 404；非法参数返回 400。
- [ ] 闪卡背面：`translation` 按「；」拆成义项逐条渲染，点击某义项可标记/取消，被标记义项加粗；点击义项不会触发翻面。
- [ ] 单词列表（WordCard）：同样支持逐义项点击标记/加粗，且点击不触发卡片其它动作。
- [ ] 标记在重新进入页面 / 重新取词后仍保留；后台查词补全覆盖 `senses` 后标记仍保留（独立列不被触碰）。
- [ ] `go test ./...` 全绿；CSV 导出、词云、统计页行为不变（不消费 `important_glosses`）。

## 未决问题

- 单词列表是否要按「是否有重要释义」做筛选 / 排序？—— 本期不做，仅在此留作后续增强。
- CSV 导出是否要在义项旁加「★」标注重要？—— 本期不做（导出走全局词典，不区分个人标记）。

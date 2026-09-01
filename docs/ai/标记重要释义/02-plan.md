<!-- 适用任务类型：feature -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-09-01 08:06 / 结束 2026-09-01 08:15 / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 实现计划

## 背景

- **任务类型**：feature
- **一句话做什么**：给 `words` 表新增 `important_glosses` 列 + `PUT /api/words/{id}/important` 接口，前端闪卡背面与单词列表把中文释义按「；」拆成义项，点击标记/取消「重要释义」并加粗显示。

## 改动方案

### 待拍板事项（对齐清单）

| 事项 | 结论状态 | 结论 / 暂缓原因 + 时间点 | 阻塞的后续任务 |
|---|---|---|---|
| 单词列表按「是否有重要释义」筛选/排序 | 暂缓 | 本期不做，等用户提出再议 | 无 |
| CSV 导出在义项旁加「★」标注重要 | 暂缓 | 本期不做（导出走全局词典、不区分个人标记），等用户提出再议 | 无 |

### 改动清单

后端（先做，逐步可编译/测试验证）：

- [x] `backend/schema.sql` —— `words` 表新增 `important_glosses JSON NULL` 列 —— 新装机建表后 `SHOW COLUMNS FROM words` 含该列
- [x] `backend/db.go` —— 新增 `migrateWordsImportantGlossesColumn()`（`columnExists` 守卫 + `ALTER TABLE words ADD COLUMN important_glosses JSON NULL`）并在 `migrateSchema` 末尾调用 —— 老库启动后该列自动补齐、重复启动无报错（幂等）
- [x] `backend/models.go` —— `Word` 加 `ImportantGlosses []string \`json:"important_glosses"\``；新增纯函数 `normalizeGlosses(in []string) []string`（trim / 去空 / 去重 / 单条 ≤200 / 总数 ≤20）—— `cd backend && go build ./...` 通过
- [x] `backend/store.go` —— `wordColumns` 追加 `important_glosses`；`scanWordRows` 扫描该列并把 NULL/空归一为 `[]string{}`；新增 `UpdateImportantGlosses(ctx, id, userID int, glossesJSON []byte) error`（`UPDATE words SET important_glosses=? WHERE id=? AND user_id=?`）—— `go build ./...` 通过
- [x] `backend/app.go` —— `wordStore` 接口加 `UpdateImportantGlosses(...)` 方法签名 —— `go build ./...` 通过
- [x] `backend/main.go` —— 注册 `PUT /api/words/{id}/important` + `handleSetWordImportant`（解析 body → `normalizeGlosses` 校验 → `FindByIDs` 校验归属 → `UpdateImportantGlosses` → 返回 `{id, important_glosses}`）—— `go build ./...` 通过
- [x] `backend/models_test.go`（或新 `normalize_test.go`）—— `normalizeGlosses` 单测（空输入 / 去空格 / 去空串 / 去重 / 超长 / 超量）—— `cd backend && go test ./...` 通过

前端（依赖后端接口，第 7 项后做）：

- [x] `frontend/src/utils/gloss.js` —— 新增 `splitGlosses(translation)`（按「；」拆分、trim、滤空）—— 由下方渲染项验证
- [x] `frontend/src/views/FlashcardView.vue` —— 背面 `.translation` 改为按 `splitGlosses` 循环渲染义项；每义项 `@click.stop` toggle、命中重要加 `gloss-important` class；toggle 做本地乐观更新 + `apiPut('/api/words/{id}/important', { glosses })`，失败回滚；scoped style 加 `.gloss-important { font-weight: 700 }` —— 闪卡翻到背面点某义项加粗、再点取消、翻面不受影响
- [x] `frontend/src/components/word/WordCard.vue` —— `.translation` 同样按义项渲染，点击 emit `set-important`（不直接改 prop）；scoped style 加 `.gloss-important` —— 单词列表点义项能加粗、不触发卡片其它动作
- [x] `frontend/src/components/word/WordList.vue` —— `defineEmits` 增加 `set-important` 并透传 `@set-important="$emit('set-important', $event)"` —— 事件能从卡片传到 HomeView
- [x] `frontend/src/views/HomeView.vue` —— `import { apiPut }`；处理 `@set-important`：乐观更新 `w.important_glosses` + `apiPut('/api/words/'+w.id+'/important', { glosses })`，失败回滚 + `ElMessage.error` —— 单词列表点义项持久化，刷新后仍保留

### 本次不做

- 单词列表按「是否有重要释义」筛选 / 排序。
- CSV 导出标注重要（★）。
- 重要标记进全局词库 / 跨用户共享（标记始终个人、只写 `words` 表）。
- 按 `pos` 区分同名义项（`v. 跑` 与 `n. 跑` 若同文本会同时加粗，个人应用可接受）。
- 孤儿标记（重查后义项文本变化导致失配）的自动清理。

## 关键文件

| 文件 | 改动性质 | 具体改哪个函数/类型/模式 | 谁调用它 / 它影响谁（callers/callees 摘要） |
|---|---|---|---|
| `backend/schema.sql` | 修改 | `words` 表加列 | 新装机建表 |
| `backend/db.go` | 修改 | 新增 `migrateWordsImportantGlossesColumn` | 被 `migrateSchema` 调用；老库升级 |
| `backend/models.go` | 修改 | `Word` 加字段 + 新增 `normalizeGlosses` | `Word` 被 `scanWordRows`/`handleAddWord`/`handleFlashcardReview` 消费（`codegraph impact Word` 13 符号） |
| `backend/store.go` | 修改 | `wordColumns`、`scanWordRows`、新增 `UpdateImportantGlosses` | `scanWordRows` 被所有取词查询调用；新方法被 handler 调用 |
| `backend/app.go` | 修改 | `wordStore` 接口加方法 | `App` 持有；`wordRepo` 是唯一实现 |
| `backend/main.go` | 修改 | 路由 + `handleSetWordImportant` | 前端 `PUT` 调用 |
| `frontend/src/utils/gloss.js` | 新增 | `splitGlosses` | 被 FlashcardView / WordCard 调用 |
| `frontend/src/views/FlashcardView.vue` | 修改 | `.translation` 渲染 + toggle | 闪卡背面 |
| `frontend/src/components/word/WordCard.vue` | 修改 | `.translation` 渲染 + emit `set-important` | 被 `WordList` 调用 |
| `frontend/src/components/word/WordList.vue` | 修改 | 透传 `set-important` | 被 `HomeView` 调用 |
| `frontend/src/views/HomeView.vue` | 修改 | 处理 `set-important` | 单词列表页 |

## 验证方式

### 验证前置 ⚠️ 写代码之前填

**命令 + 预期输出**：

```
命令：cd backend && go build ./... && go test ./...
预期：编译零错误；全部测试通过（含新增 normalizeGlosses 用例）
```

**操作路径 + 预期现象（端到端，覆盖边界行为清单）**：

1. 启动前后端并登录，首页单词列表点某个词的一个中文义项 → 该义项立即加粗；刷新页面 → 仍加粗（持久化）。
2. 再点同一义项 → 取消加粗；再点另一个义项 → 两个义项同时加粗（可多个）。
3. 进入闪卡复习，翻到背面点某义项 → 加粗，且**不触发翻面**；再点取消。
4. 对一个「旧未强化」词点「重新查询」（触发后台补全覆盖 `senses`）→ 补全完成后，之前标的重要义项仍保留（`important_glosses` 独立列不被冲掉）。
5. 后端 `curl -X PUT .../api/words/{id}/important -H 'Content-Type: application/json' -d '{"glosses":["跑"]}'` → 返回 `{"id":..,"important_glosses":["跑"]}`；`-d '{"glosses":[]}'` → `[]`；`-d '{"glosses":"x"}'` → 400；对他人词/不存在 id → 404。

### 受影响的测试

```
codegraph affected backend/store.go backend/models.go backend/main.go backend/app.go
→ ℹ No test files affected by the changed files.
```

- [x] 上述测试改动后需要重跑（`go test ./...`，确认无回归）
- [x] 前端无测试基建（`frontend/` 无 `*.test.js` / `*.spec.js`），跳过（见 `profiles/stack-detect.md` 探测结论）
- [x] 后端新增 `normalizeGlosses` 单测已作为改动清单第 7 项显式列出

## 影响面与风险

> 影响面见 `01-spec.md`「影响面」节（codegraph impact 已贴），本阶段不重复。

### 风险点

- **义项拆分依赖「；」分隔符**：若 LLM 某条 `translation` 内部含「；」会被误拆成两个义项。概率低（`mergeSensesByPos` 用「；」连接，单条义项罕见含分号），已知接受。
- **同名义项跨词性同时加粗**：`v. 跑` 与 `n. 跑` 文本相同时会一起加粗，本期不按 `pos` 区分。
- **孤儿标记**：重新查词后 LLM 返回的义项文本变化 → 旧标记失配，存了但不显示，无害、不自动清理。
- **并发连点**：全量 `PUT` 幂等，最后写入为准，无 toggle 竞态。

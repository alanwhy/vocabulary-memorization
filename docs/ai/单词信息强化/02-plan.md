<!-- 适用任务类型：feature -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-25 09:11 / 结束 2026-08-25 09:12 / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 实现计划

## 背景

- **任务类型**：feature
- **一句话做什么**：`Sense` 结构平铺新增 6 个强化字段（音标/例句/词根/词缀/近反义），查词 prompt 一并返回；录入（新增 + 再次录入）时发现词库缓存未强化则后台补全并升级缓存。

## 改动方案

### 待拍板事项（对齐清单）

| 事项 | 结论状态 | 结论 / 暂缓原因 + 时间点 | 阻塞的后续任务 |
|---|---|---|---|
| 管理员词库页是否要「未强化筛选 / 一键强化」 | 暂缓 | 本期不做，自动补全已覆盖「下次录入」场景；待实际使用中若发现存量未强化词太多、需要批量回填时再议（届时新增一个后台批量接口即可） | 无（不影响本期改动清单） |

### 改动清单

后端（先模型，再数据访问，再流程，最后前端）：

- [x] `backend/models.go` —— `Sense` 结构加 6 个 omitempty 字段（`Phonetic`/`Example`/`Root`/`Affix`/`Synonyms []string`/`Antonyms []string`），并新增纯函数 `sensesEnriched`（`len>0 && TrimSpace(senses[0].Phonetic)!=""`）—— `go test ./...` 现有测试仍绿（加字段与新增函数不破坏现有断言）
- [x] `backend/models.go` —— `mergeSensesByPos` 合并同词性时保留新字段：词级字段（phonetic/root/affix/synonyms/antonyms）取组内第一个非空值，`example` 取第一个非空值 —— 新增用例（见第 3 项）覆盖后 `go test -run TestMergeSensesByPos`
- [x] `backend/models_test.go` —— 新增 `TestSensesEnriched`（空数组→false、无 phonetic→false、有 phonetic→true、phonetic 为空白→false）与 `TestMergeSensesByPosPreservesEnrichment`（同词性合并后词级字段与 example 不丢）—— `go test -run 'TestSensesEnriched|TestMergeSensesByPosPreservesEnrichment'`（依赖第 1、2 项）
- [x] `backend/deepseek.go` —— `lookupDeepSeek` 的 prompt 改为要求返回含 6 个新字段的 JSON 数组；`parseSenses` 不再用 `Sense{Pos, Translation}` 重建丢弃新字段，改为保留 `s` 原值、仅 trim `Pos`/`Translation` —— 编译通过，配合第 5 项单测
- [x] `backend/deepseek_test.go` —— 新增 `TestParseSensesEnrichment`（解析 6 字段、容忍 markdown 包裹、空 synonyms/antonyms 数组、缺字段时回退为空）—— `go test -run TestParseSensesEnrichment`（依赖第 4 项）
- [x] `backend/store.go` —— `dictionaryRepo.SaveSenses` 的 WHERE 条件从「仅空才写」改为「空或未强化才写（升级）」，加 `JSON_EXTRACT(senses, '$[0].phonetic')` 判空 —— `go vet ./...` 通过；升级覆盖行为靠端到端验证
- [x] `backend/app.go` —— `wordStore` 窄接口新增 `MarkTranslating(ctx, id int, now time.Time) error` —— `go build ./...` 通过（依赖第 1 项，但本身独立于第 6 项，可并行）
- [x] `backend/store.go` —— `wordRepo` 实现 `MarkTranslating`（`UPDATE words SET translating=1, translation_started_at=? WHERE id=?`）—— `go build ./...` 通过（依赖第 7 项）
- [x] `backend/fakes_test.go` —— 已跳过（原因：项目实际无 fake wordStore，`wordStore` 接口仅有 `wordRepo` 一个真实实现；接口一致性已由 `go test ./...` 编译通过验证）
- [x] `backend/main.go` —— `handleAddWord`：`cacheHit && !sensesEnriched(cached)` 时 `translating=true` 并 `spawnTranslation`；`tryIncrementExisting`：解析出 senses 后若 `!sensesEnriched(existing.Senses) && !existing.Translating` 则 `MarkTranslating` + `existing.Translating=true` + `spawnTranslation` —— 端到端验证（见「验证前置」场景 1/2）（依赖第 2、3、6、7、8、9 项）

前端（依赖后端字段，可与后端联调并行开始）：

- [x] `frontend/src/components/word/WordCard.vue` —— 单词下方加音标行（`senses[0].phonetic`）、每个 sense 下加例句（`s.example`）、加词根词缀/近反义行（`senses[0]` 的 `root`/`affix`/`synonyms`/`antonyms`），均 `v-if` 条件渲染 —— `npm run dev` 下录入新词可见强化字段，旧词无字段不报错
- [x] `frontend/src/views/FlashcardView.vue` —— 翻面后与 WordCard 一致地展示强化字段 —— 闪卡翻面可见音标/例句/词根/近反义
- [x] `frontend/src/components/admin/AdminDictionary.vue` —— 「释义」列追加音标（`d.senses?.[0]?.phonetic`），其余强化字段不展开 —— 词库管理表能看到音标

### 本次不做

- 管理员词库页的「未强化筛选 / 一键强化全部未强化词」按钮（原因见「待拍板事项」，留作后续增强）。
- CSV 导出、词云、统计页不展示强化字段（它们只消费 `pos`/`translation`，保持既有口径）。
- 不引入词级独立存储（音标/词根/近反义仍平铺在 `Sense` 上重复），不做存量数据批量回填脚本——靠「下次录入」惰性补全。

## 关键文件

| 文件 | 改动性质 | 具体改哪个函数/类型/模式 | 谁调用它 / 它影响谁（callers/callees 摘要） |
|---|---|---|---|
| `backend/models.go` | 修改 | `Sense` 结构加字段；`mergeSensesByPos` 保留新字段；新增 `sensesEnriched` | 被 `deepseek.go`/`dictionary.go`/`db.go`/`main.go`/`store.go` 及 `models_test.go` 引用（impact 已列 21 个符号） |
| `backend/deepseek.go` | 修改 | `lookupDeepSeek`（prompt）、`parseSenses`（保留字段） | `parseSenses` ← `lookupDeepSeek` ← `translateWord` ← `translateAndSave`/`handleRetryDictionary` |
| `backend/dictionary.go` | 无改动（仅注释可选） | `lookupDictionarySenses`/`saveDictionarySenses` 签名与逻辑不变 | `lookupDictionarySenses` ← `handleAddWord`（唯一 caller）；`saveDictionarySenses` ← `translateAndSave`/`handleRetryDictionary` |
| `backend/store.go` | 修改 | `dictionaryRepo.SaveSenses`（SQL 条件）；新增 `wordRepo.MarkTranslating` | `SaveSenses` 由 `saveDictionarySenses` 调用；`MarkTranslating` 由 `tryIncrementExisting` 调用 |
| `backend/app.go` | 修改 | `wordStore` 接口加 `MarkTranslating` | 接口由 `wordRepo` 与 `fakes_test.go` 实现 |
| `backend/main.go` | 修改 | `handleAddWord`、`tryIncrementExisting` 加补全触发 | `handleAddWord` callees：`tryIncrementExisting`/`lookupDictionarySenses`/`Insert`/`spawnTranslation`；`spawnTranslation` 另有 `resumeStuckTranslations`/`sweepStuckTranslations` 两个 caller（复用管线不受影响） |
| `backend/fakes_test.go` | 修改 | fake `wordStore` 补 `MarkTranslating` | 供全部 handler 测试编译 |
| `frontend/src/components/word/WordCard.vue` | 修改 | 模板加音标/例句/词根/近反义渲染 | 被 `WordList` 使用 |
| `frontend/src/views/FlashcardView.vue` | 修改 | 翻面 back 面加强化字段渲染 | 独立路由页 |
| `frontend/src/components/admin/AdminDictionary.vue` | 修改 | 释义列加音标 | 被 `AdminView` 使用 |

## 验证方式

### 验证前置 ⚠️

**命令类**：

```
命令：cd backend && go vet ./... && go test ./...
预期：vet 无告警；全部单测通过（含新增的 sensesEnriched / mergeSensesByPos 保留字段 / parseSenses 新字段 用例）
```

```
命令：cd frontend && npm run build
预期：构建成功（三个组件模板语法正确，无未定义变量）
```

**端到端操作路径类**（本地环境已起：后端 :8080 / 前端 :5173 / MySQL `vocab-mysql-dev`）：

1. **新词强化（happy path）**：实现后重启后端，登录后录入一个词库缓存里没有的新词（如 `serendipity`）→ 录入即返回 `translating=true` → 等待几秒 → 单词卡出现「音标 + 词性 + 释义 + 例句 + 词根词缀 + 近反义」。对应边界项「空/无数据」「接口契约变更」。
2. **历史词补全（核心需求）**：当前库里 `apple` 的缓存还是旧格式 `[{"pos":"n.","translation":"苹果；苹果树；苹果公司"}]`（无 phonetic）。重启后再次录入 `apple` → 走 `tryIncrementExisting` → 判定未强化 → 后台补全 → 等几秒 → `apple` 卡片补出音标/例句等，且 `word_dictionary.apple` 的 senses 被升级。对应边界项「历史数据兼容」。
3. **已强化词不重复查**：场景 2 补全完成后，再录入一次 `apple` → 观察后端日志不再新增 DeepSeek 调用（命中强化缓存）。对应「并发/幂等」。
4. **快速连点不重复查**：对场景 2 的未强化词，极短时间内连点两次「添加」→ 观察后端日志只触发一次 DeepSeek 查询（`translating` 标志防重）。对应「并发/重复请求」。
5. **失败占位不因新字段报错**：临时把 DeepSeek 配置的 key 改错，录入新词 → 仍落库、显示「查词中…」→ 重试耗尽后落「系统提示：查询失败」，前端不因缺新字段报错。对应边界项「异常状态」。

### 受影响的测试

`codegraph affected` 对改动的后端文件返回「No test files affected」（该命令未把 `_test.go` 对 `Sense` 的引用计入）；但据 spec 阶段 `codegraph impact "Sense"` 输出，实际受影响的测试为：

- `backend/models_test.go` — `TestMergeSensesByPos` / `TestMergeSensesByPosSkipsEmptyTranslation` / `TestMergeSensesByPosPreservesFirstOccurrenceOrder`
- `backend/dictionary_test.go` — `TestFormatSenses` / `TestFormatSensesWithoutPos` / `TestFormatSensesEmpty`
- `backend/fakes_test.go` — 因 `wordStore` 接口新增 `MarkTranslating` 而需补实现（否则测试包编译失败）

- [x] 上述测试改动后需要重跑（`go test ./...`）
- [ ] 本项目无测试基建，跳过（不适用：项目已有 46 个单测）

## 影响面与风险

### 风险点

- **`SaveSenses` 升级覆盖的并发语义**：若把 WHERE 条件写错（无条件 UPDATE），并发下可能用旧数据覆盖已强化的缓存。已用「仅当空或未强化才写」的条件兜住，实现时需严格照 02b-design 的 SQL。
- **`mergeSensesByPos` 丢字段**：现在用 `Sense{Pos, Translation}` 重建会静默丢掉新字段，必须改；否则补全后写回的仍是残缺数据。
- **`tryIncrementExisting` 防重**：不加 `translating` 判断会在快速连点时重复调用 DeepSeek；`MarkTranslating` 必须在 `spawnTranslation` 之前落库。
- **前端 `senses[0]` 越界**：`senses` 为空数组时 `senses[0]` 为 undefined，所有新字段渲染必须用 `v-if="senses && senses.length"` 包住，或 `senses[0]?.phonetic` 可选链。
- **`parseSenses` 对 LLM 返回 `synonyms`/`antonyms` 为字符串而非数组的容错**：prompt 已明确要求数组，但模型偶发返回字符串会导致 JSON 反序列化失败；如需更稳可在 `parseSenses` 里做一次数组归一化（实现时视实际返回决定，不提前过度设计）。

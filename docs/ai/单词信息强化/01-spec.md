<!-- 适用任务类型：feature -->
<!-- 阶段记录：模型 deepseek/deepseek-v4-pro / 开始 2026-08-25 08:47 / 结束 <YYYY-MM-DD HH:mm> / 本阶段 token（in/out）未记录（VSCode 扩展内无 /cost）/ 本阶段成本 未记录 / 规范版本 team-dev-skills@1.0.0 -->

# 单词信息强化：查词返回音标/例句/词根词缀/近反义词，缺失时下次录入自动补全

## 需求陈述

当前查词只返回「词性 + 中文释义」两个字段。用户希望强化单词信息：查词时让大模型一并返回音标（phonetic）、例句（example）、词根词缀（root/affix）、近义词/反义词（synonyms/antonyms），并在前端展示。

对于升级前已经查过、只缓存了「词性+释义」的历史词条，这些强化字段是缺失的。用户要求：**当再次录入这类词时，如果发现强化内容缺失，就调用大模型后台补全**，而不是永久停留在信息不全的状态。

## 定制 or 通用 ⚠️ 必填

- [x] **通用能力** —— 受益范围：本应用的所有用户（这是一个个人单词本应用，无多客户/多租户概念）

说明：本仓库是单部署的 Go 后端 + Vue 前端应用，**没有物理的「公共层 / 定制层」分层**。`.claude/ai-profile.md` 刚由引导脚本生成、尚未识别出任何公共层/定制层路径（layer-detect 扫描无 `common/shared/customers/...` 命名的目录）。因此「定制 or 通用」判定直接落「通用能力」，无公共层下沉风险。

## 影响面

`codegraph impact "Sense"` 的输出（21 个受影响符号）：

```
backend/models.go      struct Sense:10 / func mergeSensesByPos:16
backend/deepseek.go    func parseSenses:92 / func lookupDeepSeek:36
backend/dictionary.go  handleRetryDictionary / lookupDictionarySenses / saveDictionarySenses / formatSenses / handleExportDictionary
backend/db.go          mergeHistoricalWordSenses / backfillWordDictionary
backend/models_test.go TestMergeSensesByPos* (3 个)
backend/main.go        translateAndSave / handleAddWord / saveWordSenses
backend/store.go       cloudMeaning
backend/dictionary_test.go  TestFormatSenses* (3 个)
```

`codegraph impact "mergeSensesByPos"` 额外指向 `spawnTranslation`、`migrateSchema`、`Stats`。

- **已确认事实**：上面的 codegraph 原始输出。
- **推断**：`Sense` 是核心实体，改动会波及后端约 8 个文件 + 前端 3 个组件；其中 `models.go` / `deepseek.go` / `store.go` 属于「通用工具 / 数据转换」层，按验证要求分层表落到「要求/建议测试」档。

## 依赖服务现状 ⚠️ 涉及外部服务

| 依赖 | 方法签名 | 现有方法是否满足 | 是否需要新增/变更 | 依据 |
|---|---|---|---|---|
| DeepSeek `/chat/completions` | `lookupDeepSeek(ctx, word, cfg deepseekConfig) ([]Sense, error)` | 满足（接口不变） | 否，只改 prompt 与解析 | 已确认（读了 deepseek.go） |

仅此一个外部依赖；查词入口 `translateWord` 与后台管线 `spawnTranslation → translateAndSave` 均已存在，直接复用，无需新增服务或方法签名。

## 核心语义与数据模型 ⚠️ 必填

- **Sense 结构** `[已确认]`：`senses` 是 JSON 数组，同时存在于 `words.senses` 与 `word_dictionary.senses` 两列。本次采用「全部平铺进 Sense」方案（用户已确认），结构扩展为：

  ```go
  type Sense struct {
      Pos         string   `json:"pos"`
      Translation string   `json:"translation"`
      Phonetic    string   `json:"phonetic,omitempty"`   // 音标
      Example     string   `json:"example,omitempty"`    // 例句（词义级）
      Root        string   `json:"root,omitempty"`       // 词根
      Affix       string   `json:"affix,omitempty"`      // 词缀
      Synonyms    []string `json:"synonyms,omitempty"`   // 近义词
      Antonyms    []string `json:"antonyms,omitempty"`   // 反义词
  }
  ```

  词级字段（phonetic/root/affix/synonyms/antonyms）在多词性下会重复存储；前端只取第一条词性的词级信息展示。**不需要任何表结构变更或数据迁移**——旧数据只是少了这些可选 key，仍是合法 JSON。

- **关键 ID 含义** `[已确认]`：`wordKey = strings.ToLower(displayWord)`，是 `words`（按 user_id 唯一）与 `word_dictionary`（按 word_key 唯一）两表的关联键。查词与补全都以 `wordKey` 为准。

- **核心数据流转** `[已确认]`：录入 → 先查 `word_dictionary` 缓存（`lookupDictionarySenses`）→ 命中且有释义则复用，否则后台 `spawnTranslation → translateAndSave` 调 DeepSeek，把结果同时写回 `words.senses`（`saveWordSenses`）和 `word_dictionary.senses`（`saveDictionarySenses`）。

- **「缺少内容」的判定** `[已确认，本次新增]`：新增纯函数 `sensesEnriched(senses []Sense) bool`，判据为 **`len(senses) > 0 && senses[0].Phonetic != ""`**。理由：音标是每个英文单词都必有、而旧 prompt 一定没返回的字段，是最可靠的「是否已强化」信号；root/affix/synonyms/antonyms 对某些词天然为空，不能作为判据。

- **多个入口行为一致性** `[已确认]`：`formatSenses`（导出）、`cloudMeaning`（词云）、前端三处渲染都只消费 `pos/translation`，强化字段是纯增量、不影响既有显示。管理员「重新查询」接口 `handleRetryDictionary` 复用 `translateWord`，prompt 改动后自动产出强化结果。

- **数据相关的边界** `[已确认]`：并发重复触发补全需防重（见边界行为清单）；`saveDictionarySenses` 目前「仅当缓存为空才写」，补全场景需要「仅当新数据已强化、旧数据未强化时才覆盖升级」。

## 设计要点

1. **平铺进 Sense，零迁移**。用户已确认。相比「词级独立对象 / 独立列」两个方案，它改动最小、不需要动表结构或迁移存量 JSON；代价是词级字段重复存储，个人应用可接受。

2. **补全复用整条查词管线**。`spawnTranslation → translateAndSave` 已具备：信号量限流、可取消、退避重试、卡死扫描。补全只是「在命中缓存但判定未强化时，把它当作一次未命中来走管线」，不新起一套机制。

3. **补全的触发点（两处，均后台异步）**：
   - `handleAddWord`：`cacheHit` 为真但 `sensesEnriched(cached)` 为假 → `needsEnrichment=true`，`translating=true`，`spawnTranslation`。
   - `tryIncrementExisting`：再次录入同一词时，若 `existing` 已解析出的 senses 未强化且不在查词中 → `spawnTranslation` 补全。

4. **写回语义升级**。`saveDictionarySenses` 需要在「升级补全」时也能覆盖：改为「当新 senses 已强化、且库里现存 senses 未强化时才覆盖」，既能让历史词条被补全，又避免并发下把已强化的好数据覆盖回旧数据。落库用 `WHERE senses 未强化` 的条件更新，而不是无条件 UPDATE。

5. **prompt 与解析同步升级**。`lookupDeepSeek` 的 prompt 改为要求返回含六个新字段的 JSON 数组；`parseSenses` 不再丢弃新字段（保留 `s` 原值，仅 trim pos/translation）。`mergeSensesByPos` 合并同词性时，词级字段取组内第一个非空值、example 取第一个非空值，避免合并后字段丢失。

6. **被否方案记录**：
   - 「词级独立对象（senses 改成 `{phonetic, senses:[...]}`）」—— 干净但不做，因为要迁移存量 JSON、改动 `mergeSensesByPos`/`formatSenses`/`backfill`/`cloudMeaning` 等多处，收益对个人应用不成比例。
   - 「同步阻塞查词」—— 不做，录入体验差且与现有异步+轮询机制相悖。

## 可复用资产 ⚠️

| 资产类型 | 已发现的候选 | 是否复用 | 不复用的原因 |
|---|---|---|---|
| 查词管线 | `spawnTranslation` / `translateAndSave` / `translateWord` | ✅ 复用 | — |
| 缓存读写 | `lookupDictionarySenses` / `saveDictionarySenses` | ✅ 复用（save 需加升级写回条件） | — |
| 数据转换 | `mergeSensesByPos` / `parseSenses` / `formatSenses` | ✅ 复用（前两个需扩展新字段） | — |
| 前端渲染 | `WordCard.vue` / `FlashcardView.vue` / `AdminDictionary.vue` 的 senses 渲染 | ✅ 复用（追加新字段展示） | — |
| 表结构 | 无 `sql/upgrade/`，schema 由 `db.go` 的 `migrateSchema` + `schema.sql` 管理 | 不涉及 | 平铺方案零表结构变更 |

## 边界行为清单 ⚠️ 必填

- [x] **空 / 无数据场景**：词库缓存为空或查词彻底失败 → 沿用现有 `failedSenses` 占位与 `translating` 轮询机制，不变。补全判定用 `len(senses)>0` 前置，空 senses 直接走原「未命中」分支。
- [x] **异常状态（网络失败 / 大模型报错）**：补全沿用 `translateAndSave` 的退避重试与失败占位，不因新增字段改变现有错误语义。
- [x] **历史数据 / 存储结构变更**：无表结构变更、无迁移。历史词条因缺 `phonetic` 被判定为「未强化」，下次录入时自动补全，无需批量回填脚本。
- [x] **原有 mock / 测试替身 / 本地持久化**：`fakes_test.go` 里 `dictionaryStore` 接口若新增方法，需同步给 fake；尽量不加新接口方法、复用 `SaveSenses` 改语义即可避免。
- [x] **接口契约变更**：`Sense` 的 JSON 字段新增（纯增量、omitempty），前端 TS 类型与三处组件渲染同步扩展。契约变更见 `02b-design.md`「接口契约」块。
- [x] **权限 / 角色差异**：查词补全对任意登录用户录入行为生效；管理员「重新查询」接口不变。无租户差异。
- [x] **并发、重复请求、幂等性**：再次录入快速连点可能对同一词并发触发多次补全。需防重——复用 `translating` 标志（补全开始时置 1、写回时 `UpdateSenses` 已置 0），或内存级 wordKey 去重。具体方案在 `02b-design.md` 定，确保同一词不重复调用大模型。

## 验收标准

- [ ] `Sense` 结构含 6 个新字段，且 `json` tag 为 omitempty；`words.senses` / `word_dictionary.senses` 无需迁移即可容纳。
- [ ] 新查词（`lookupDeepSeek`）返回的结果含音标/例句/词根词缀/近反义词，`parseSenses` 不再丢弃新字段（有单测覆盖）。
- [ ] `sensesEnriched` 纯函数：空数组 → false；非空但第一条无 phonetic → false；有 phonetic → true（有单测）。
- [ ] 录入一个「词库已缓存旧版释义」的词 → 返回 `translating=true` 且后台补全，完成后词条带上强化字段（可人工用真实 DeepSeek 验证或用手写 fake 验证管线）。
- [ ] 再次录入一个「个人列表里已存在、但未强化」的词 → 后台补全触发，且快速连点不产生重复大模型调用。
- [ ] 前端 WordCard / Flashcard / AdminDictionary 能展示音标、例句、词根词缀、近反义词；无强化字段时展示不报错、不显示空壳。
- [ ] 现有单测全部通过（`go test ./...`）；`mergeSensesByPos` 扩展后旧用例仍绿，新增「合并时保留新字段」用例。
- [ ] CSV 导出、词云、统计页行为不变（只消费 pos/translation）。

## 未决问题

- 管理员词库页是否要暴露「未强化 / 已强化」筛选，或提供「一键强化全部未强化词」按钮？—— 本期默认不做（自动补全已覆盖「下次录入」场景），仅在此留作后续增强项，如需一并做请在 `/sw-plan` 前告知。

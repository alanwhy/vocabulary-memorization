当前需求已变更为每日临时练习，最新设计与本轮实现依据见 [04-simplification.md](04-simplification.md)。浩原已授权直接简化并确认同日下一批继续取后 10 个。

执行模式：分阶段确认（来源：浩原先要求调研；2026-11-12 进一步要求“开始做吧，给我一个完成的方案”，本轮完成方案定稿、详细设计与实现计划，不自动接续模块编码）

阶段授权追加：2026-11-12 浩原明确要求“可以，开始实现”，进入本任务实现阶段；实现开始记录 09:01:40（Asia/Shanghai）。沿用减少独立复核、手动验收要求；提交、部署和飞书归档不在本次授权内。

# 同义替换记忆模块调研与候选方案

阶段记录：模型 GPT-6（具体变体未提供）；日期 2026-11-12（Asia/Shanghai）；项目 frontend@1.23.0；规范 @sw/dev-skills@1.3.3。

方案已整理完成：[完整产品方案](README.md)、[详细设计](02b-design.md)、[实现计划](02-plan.md)。以下调研、候选方案和早期建议保留为设计依据；第一版的确定范围、默认行为、接口与存储以完整产品方案和详细设计为准。英语一/英语二通过模块内选择解决，第一版默认“考研通用”；内容默认个人隔离；联网自动搜集真题后续独立扩展，第一版支持粘贴材料与来源链接。

## 需求陈述

浩原希望通过近义词记忆，提升阅读理解中识别原文与选项同义替换的能力。目前尚未确定学习形式，先调查学习方法与项目可复用能力，比较候选方案。

目标应覆盖三个层次：认出常见替换表达；判断某个语境下是否能替换；判断整句改写是否保留原意。训练结果不能只用背了多少近义词衡量。

浩原已确认目标为考研英语，英语一或英语二尚未区分。引用雅思教学资料仅作为语境理解与改写方法的依据，不把雅思题型直接套用为考研模块。

考研方向的产品建议：将熟词僻义、短语替换与保留原意判断放在核心；同义表达卡作为练习后的辅助解释。先做短句微训练，后续扩展到短段落中的观点、因果和转折。上述为设计建议，本轮未系统统计历年考研真题的替换频率，不能将候选表达标为已验证的高频真题词。

建议分三类准备内容：

- 义项与短语：如 account for the decline ↔ explain the decrease；account for 30% ↔ make up 30%。同一短语按语境拆开，不能共用一个无条件同义关系。
- 句子改写：如 The report accounts for the decline in sales. ↔ The report explains why sales fell. 核对主语和逻辑作用，不只看重合词。
- 干扰辨析：可能改成一定、部分改成全部、解释原因改成造成结果、作者观点改成他人观点。短句第一版先覆盖可清晰验证的范围、可能性和因果角色，观点归属留到短段落练习。

## 已确认的项目现状

| 能力 | 当前实现与依据 | 对新模块的意义 |
|---|---|---|
| 近义词与例句 | `backend/internal/model/models.go:21` 的 Sense 包含 synonyms、example；`backend/internal/app/deepseek.go:45` 起的查词提示词生成这些内容 | 可以寻找候选表达，不足以直接形成有唯一答案的试题 |
| 当前近义词是词级信息 | 提示词要求每个词性的 synonyms 一致；同词性多个释义合并；`frontend/src/components/word/WordCard.vue:39` 起从第一条释义取关系词 | 缺少“哪一个义项、哪个搭配里能替换”的约束，尤其要处理多义词 |
| 闪卡与间隔排期 | `frontend/src/composables/useFlashcardQueue.js`；`backend/internal/model/models.go:185` 的 ApplySRSScheduling | 可以复用学习节奏与排期算法；关系掌握度应单独记录 |
| 现有记住行为 | `backend/internal/app/words.go:518`：good 直接归档，队列只取未归档到期词 | 不能把新模块答对直接转成旧闪卡 good；会误把认识原词等同于掌握所有替换关系 |
| 查词、缓存、多用户 | 个人 words、全局 word_dictionary，现有登录与用户隔离 | 可共享校验后的内容，个人训练进度与个人摘录要隔离 |
| 页面入口 | `frontend/src/router/index.js`：首页、归档、统计、闪卡、个人中心、后台 | 当前没有阅读训练模块；新模块可以独立入口呈现 |

本轮没有调用实际模型生成题库，没有检查生产数据的近义词覆盖率，也没有验证已有词库内容准确率。

## 调研依据与设计启示

### 原文与选项替换不止换一个词

[British Council：阅读中的上下文线索](https://takeielts.britishcouncil.org/blog/mastering-context-clues-boost-your-ielts-reading-skills)直接建议练习阅读句子改写，并给出词与短语间的替换，如 exhausted / worn out。它还强调借助上下文理解词义。

[British Council：五种改写方法](https://www.britishcouncil.vn/en/adults/tips/top-five-paraphrasing-techniques-video-included)说明近义词、词形变化、主动被动、语序变化及组合使用。该文主要面向写作，支持“改写有哪些形式”，不直接证明某种阅读训练模块的效果。

设计启示：先练词与短语替换，再逐步扩展到句法改写；用例句限定义项。比如 address a problem 可对应 deal with a problem，address an envelope 则不能这样替换。

### 练习需要主动判断，不能只有列表浏览

[Roediger 与 Karpicke（2006）：Test-enhanced learning](https://profiles.wustl.edu/en/publications/test-enhanced-learning-taking-memory-tests-improves-long-term-ret/)使用短文与自由回忆测试，延迟测验中测试条件的保持效果优于重复阅读。研究支持主动提取的价值，但不能把自由回忆的实验结论直接等同于“四选一近义词题必然有效”。

设计启示：先作答再显示解释；选择题后加入对应表达辨认；熟练后减少高亮与提示。阅读识别是第一版主目标，英文拼写产出可作为可选进阶，而非默认负担。

### 把大量近义词一起背可能增加混淆

[Nakata 与 Suzuki（2019）：语义相关词与集中/间隔学习](https://www.cambridge.org/core/journals/studies-in-second-language-acquisition/article/effects-of-massing-and-spacing-on-the-learning-of-semantically-related-and-unrelated-words/F58BA8D70385603B9C42E408BFCB8A10)研究了 133 名日本大学生学习 48 组英日词对。相关词与非相关词的后测分数无显著差异，但相关词产生更多干扰错误；间隔学习对非相关词的收益反而更大。

因此不能宣传“按近义词成组背一定更高效”，也不能声称间隔重复可以彻底消除同义词混淆。建议以已经认识的词作锚点，每次引入少量替换，配合具体语境和混合练习。这是产品假设，需要实际试用验证。

## 四种候选方案

| 方案 | 如何学习 | 主要价值 | 局限 | 相对开发量 |
|---|---|---|---|---|
| A：同义表达卡 | 按某个义项展示 2–3 个替换表达、对比例句与使用区别；翻面前先回想 | 积累表达，使用简单，可作为解释页 | 只浏览容易觉得眼熟；向阅读选项判断的迁移较弱 | 较小 |
| B：语境替换练习 | 给一句话和目标表达，从候选中选出在此处能替换的表达；答后解释 | 把词义、搭配与句意联系起来 | 每题必须确保唯一答案；仅靠题库重做可能记住答案 | 中等 |
| C：原文—选项微训练 | 给 1–3 句原文和选项，判断哪项保留原意，再辨认替换对应 | 最接近目标技能；能训练范围、语气、因果与主体辨析 | 出题质量要求更高，阅读负担也更大 | 中等偏大 |
| D：个人阅读摘录本 | 粘贴遇到的原文、题目选项，提取替换关系，经确认后加入复习 | 直接围绕自己的材料与错题积累 | 需要持续收集材料；自动识别需要纠错入口 | 完整版较大；手工成对录入较小 |

相对开发量是根据内容生成、验证与进度管理需求的初步判断，不是工时承诺。

### A 的具体例子

义项：reduce = 减少。

- reduce energy costs ↔ lower energy costs / cut energy costs。
- 数量变化和程度需要上下文判断；不能把 reduce 与表示大幅削减的表达无条件画等号。
- 只展示少量适用于当前例句的关系，不生成可传递的“全都互换”词群。

### B 的具体例子

原句：The committee decided to postpone the meeting.

在保留句意并允许必要语法调整的条件下，postpone the meeting 对应 put off the meeting。干扰项可以是 cancel the meeting、bring forward the meeting、hold the meeting。

作答后揭示：postpone ↔ put off；延后与取消、提前、召开不同。练习题不能把语法允许调整的语义改写与“原位替换一个词”混为一谈。

### C 的具体例子

原文：The new policy is likely to reduce energy costs for households.

正确改写：Households will probably spend less on energy because of the new policy.

对应：is likely to ↔ will probably；reduce energy costs ↔ spend less on energy。主体与原因保持一致。

错误选项分别只改变一个关键维度：把 probably 改成 certainly；把 households 改成 businesses；把 spend less on energy 改成 use less energy。解析明确指出“确定性增强”“主体改变”“费用被换成用量”。

这是一道原创方法演示题，不是真题，不表示整套题库已经生成或验证。

### D 的具体例子

保存一条关系：原文中的表达、选项中的表达、两者所在完整句子、对应义项、为什么该题能替换、材料来源。

让用户确认 AI 提取的关系。不要默认整段原文与所有选项都同义；错误选项也可能包含局部正确替换。

## 第一版方向

结合已确认的考研英语目标，以 B 建立表达基础，C 作为主要训练方向，A 作为新关系的学习卡与答后的解释卡。第一版加入 D 的粘贴材料导入与替换提取，联网搜集留待后续。默认先学最多 3 条新关系再练，用户可以选择直接练习；学习顺序不再作为实现阻塞。

建议使用一个清晰入口“同义替换”，提供“学习表达”和“阅读练习”两种体验。第一版一次约 10 题，5–8 分钟是目标节奏，实际耗时由试用确认。

一个学习回合：

1. 从本人已录入的词中选锚点，优先用户标记的重要释义；找不到可靠义项时不猜测。
2. 初次学习每个义项只引入少量替换，展示语境与区别。
3. 做语境选择题，再做短句保留原意判断；训练中暂时隐藏中文释义和答案映射。
4. 答后揭示表达对应与每个错误选项的改动。查看提示的作答不算独立掌握。
5. 错误与不确定的关系进入个人复习；后续更换句子、选项顺序与问法。

可以优先在已知词与新表达之间建立关联；不能直接让用户一次背一串互不认识的近义词。词群浏览或关系图可作为查询方式，暂不作为主要训练入口。

## 核心语义与数据模型

下列条目保留概念模型说明；已确定的表结构、数据隔离与接口见 02b-design.md。

- 已确认：现有 word_key 是归一化单词标识，个人 Word.ID 属于个人词条，全局词典内容与个人学习数据分开。
- 建议：关系 ID 应指向“表达 A + 表达 B + 具体义项 + 上下文条件”，不能只由两个单词构成。
- 建议：保存词性、义项说明、原句、改写句、替换跨度、使用限制、关系来源与内容版本。短语允许多词。
- 建议：题目引用关系与版本，包含题型、答案和逐项解释；训练请求只返回作答所需内容，答案在提交后揭示。
- 建议：个人掌握度按用户与关系记录；实际作答历史区分错误、猜中、用了提示与独立答对。不要只存 word.review_count。
- 已定：第一版所有题目、关系与材料按用户隔离，同一用户复用内容缓存；公共题库共享后续再议。
- 已定：未归档词、已归档词都可作为锚点，因为认识单词与认识替换关系是两项能力。
- 多入口：单词详情和新模块可打开同一关系解释；阅读练习不触发旧闪卡归档。新旧模块学习统计应能区分。

## 内容来源与质量

| 来源 | 适用方式 | 取舍 |
|---|---|---|
| 当前近义词与例句 | 提供候选，不直接作为答案依据 | 最贴合现有词库，但缺少义项与替换限制 |
| 人工准备的小规模内容 | 第一版种子题，用于试用 | 可控；数量有限，需维护 |
| AI 生成并缓存 | 从本人词汇按需生成关系和例题 | 个性化；需要结构、语义与唯一答案核验，缓存控制成本与等待 |
| 本人导入材料 | 提取与确认真实遇到的替换 | 相关性高；来源和个人可见性需要记录 |
| 外部词典/词库 | 核验用法或提供候选 | 本轮未选定供应商；上线前需确认获取方式、许可与义项质量 |

推荐先用可人工检查的少量内容试用，再考虑按需生成。独立模型核验只能降低错误概率，不能保证题目正确；用户应能反馈“多个选项都对”“解析不对”，争议题不计入掌握度并暂停出题。不能把低温生成当作事实校验。

## 适用范围与改动落点

属于本应用通用学习功能。若采用推荐方案，预计新增独立页面、练习状态管理、题目/关系内容服务与个人训练存储，路由及导航增加入口。

复用登录、查词基础设施及排期算法的思路。既有词义模型与旧闪卡状态耦合较广，优先建立独立关系内容与进度，而非把题目塞进词级 synonyms 字符串。

`.claude/ai-profile.md` 仍为未填写的模板，列出的公共层路径不是项目已确认的边界。本轮仅新增调研文档及会话内演示，不修改公共逻辑。

## 影响面证据

本轮先使用 CodeGraph 查明现状，没有自动创建或修改索引。以下是定位候选复用点的原始影响输出；不表示计划修改这些符号。

```text
Impact of changing "useFlashcardQueue" — 6 affected symbols:

frontend/src/composables/useFlashcardQueue.js
  function    useFlashcardQueue:114

frontend/src/views/ArchiveView.vue
  function    handleCorrected:18
  file        ArchiveView.vue:1

frontend/src/views/HomeView.vue
  function    handleCorrected:200
  file        HomeView.vue:1

frontend/src/views/FlashcardView.vue
  file        FlashcardView.vue:1

Impact of changing "Sense" — 32 affected symbols:

backend/internal/model/models.go
  struct      Sense:21
  function    SensesEnriched:38
  function    SensesNeedEnrichment:49
  function    mergeSenseEnrichment:59
  function    MergeSensesByPos:91

backend/internal/app/model_aliases.go
  function    mergeSensesByPos:19
  function    sensesEnriched:23
  function    sensesNeedEnrichment:27

backend/internal/app/dictionary.go
  method      handleRetryDictionary:114
  method      lookupDictionarySenses:23
  method      saveDictionarySenses:40
  function    formatSenses:52
  method      handleExportDictionary:151

backend/internal/app/words.go
  method      translateAndSave:186
  method      handleAddWord:27
  method      saveWordSenses:255

backend/internal/app/correction.go
  method      handleCorrectWord:61
  method      ensureWordEnrichment:29
  function    separateWordRelations:158

backend/internal/app/word_enrichment_test.go
  function    TestFlashcardEnrichmentCacheAndNoRepeatedLookup:249
  function    TestRepeatedInputReusesEnrichedCache:280
  method      TranslationSenses:75

backend/internal/app/dictionary_test.go
  function    TestFormatSenses:5
  function    TestFormatSensesWithoutPos:14
  function    TestFormatSensesEmpty:23

backend/internal/model/models_test.go
  function    TestSensesEnriched:77

backend/internal/app/deepseek.go
  function    lookupDeepSeek:37
  function    sanitizeSenses:125
  function    parseSenses:147
  function    parseLookupResponse:158

backend/internal/app/translate.go
  function    translateWord:19

backend/internal/storage/words.go
  method      TranslationSenses:453
```

## 依赖与可复用资产

| 资产/服务 | 现有签名或用途 | 候选复用与限制 |
|---|---|---|
| 查词 | lookupDeepSeek(ctx context.Context, word string, cfg deepseekConfig, reason ...string) ([]Sense, bool, error) | 仅用于候选词义；训练生成应独立约束，不重写整个查词提示词来顺带出题 |
| 排期计算 | model.ApplySRSScheduling(intervalDays int, easeFactor float64, rating string) (int, float64) | 可复用纯计算；新模块评价与旧评分的映射待定，不复用直接归档副作用 |
| 当前闪卡提交 | POST /api/flashcards/review，{id, rating} | 不满足关系级训练需求，不直接调用 |
| 全局词典 | DictionaryStore.LookupSenses(ctx context.Context, wordKey string) ([]byte, error) | 可找候选；没有义项级关系与题目存储 |
| 个人重要释义 | Word.ImportantGlosses | 可作为锚点筛选线索；文本变化、一个词性多义时仍需确认义项 |

已检查 `backend/internal/storage/migrations.go` 与 `backend/schema.sql`，现有迁移为启动时幂等执行；本仓库没有 `sql/upgrade/` 目录。新增存储与兼容方式见 02b-design.md。

## 边界行为清单（候选要求）

- 空/无数据：没有可靠关系时提示尚无可练内容，提供准备内容或录入入口。候选不足时按实际数量出题，不凑出歧义题。
- 异常：生成失败、超时、格式不完整与候选不足分开说明；失败保留既有题目和学习进度，不显示成“没有数据”。
- 历史兼容：旧 synonyms 仅作候选；缺字段、空列表和查询失败占位不能直接进入训练；补内容不重置原词进度。
- 本地演示：会话演示题是原创固定内容，不保存真实学习进度，不计入应用统计；真实模块需服务端保存个人进度。
- 契约：新练习接口与旧查词、闪卡接口分离；本轮未确定接口字段、供应商与表结构。
- 权限：所有角色可学习，个人进度及私人导入材料按用户隔离；若做公共内容管理，则管理权限另行确定。
- 并发：重复生成按关系/版本去重；重复提交同一次作答应幂等，不累计两次成绩；具体机制下一阶段确定。
- 语义：同义关系依赖义项与语境，不做跨词群传递；句意判断核对主体、时间、数量范围、程度、否定、可能性与因果方向。
- 唯一答案：多个正确选项或证据不足的题暂停使用；纠错后的题目版本与历史作答区分。
- 提示泄露：先作答后显示中文与对应表达；提示后答对单独标记，初级目标表达高亮可在进阶关闭。
- 复习：避免只重复同一句；用换语境后的成绩观察掌握程度，不以一次选择正确自动判定掌握。
- 中途退出：保留本组位置与已成功提交的结果；提交失败可重试，不推进到下一题。
- 删除与纠错：本人删除原词不删除他人共享内容；关联关系是否保留由模块策略决定。词义纠错影响的题目需要重新核验，而非静默沿用旧答案。

## 候选验收与效果观察

最终验收要随方案选择调整，当前仅用于讨论：

- [ ] 能用具体义项和语境解释每条替换关系，包括短语。
- [ ] 练习先作答后解释，每题有可判断的唯一答案与干扰项理由。
- [ ] 同一关系可以在不同句子中练习，选项顺序变化不改变答案语义。
- [ ] 训练结果、提示使用与复习进度仅影响当前用户；答对不自动归档旧词。
- [ ] 多义词、可能/一定、部分/全部、费用/用量这类错误能被识别并解释。
- [ ] 生成失败或争议题不计入正确率，既有进度不丢失。
- [ ] 手动试用检查学习节奏、阅读负担、题目自然程度与解释准确性。

可安排小规模试用：先测未见过的句子，训练一段时间后使用另一组未见句子复测，再做延迟回测。记录正确率、错误维度和用提示比例，观察是否只记住原题。不据此承诺考试提分；具体题量和周期由浩原决定。

## 关键选择的收敛结果

1. 考研英语通用为默认范围，英语一/英语二和难度由模块内选择，不为此阻塞方案。
2. 默认先学习少量新表达，再做短句训练；提供直接练习，支持偏好不同的用户。
3. 本人词库生成与手工导入真题素材都纳入第一版；没有词库时可以输入自选词作生成种子。
4. 完整学习、答题、解析、复习、材料导入和反馈闭环纳入第一版；自动联网搜集真题、整套试卷模拟与公共题库暂缓。

本轮交付完整方案和实现计划。文档验收与模块实现验收分开，现阶段没有模块业务验收结果。

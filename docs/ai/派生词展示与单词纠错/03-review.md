<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-21 08:01 / 结束 2026-10-21 08:12 / 规范版本 @sw/dev-skills@1.1.2 -->

# 自审与验收

## 结论

**通过。** 当前实现没有未修复的确定性 critical 问题。派生词增量补齐、完整纠错、失败恢复和本地轮询顺序保护均与最终设计一致，计划验证前置全部已实际执行。最后一轮旧闪卡页面采用独立测试响应快照，已确认由待补齐自动显示非空派生词且保持翻面、1/1、×7；先前测试 API 共享对象造成的证据缺口已闭环。结论限于本地源码、自动测试及测试 API 页面验证，不等同真实 MySQL、模型语义或生产验收。

本次独立复核读取了 `git diff`、空的 `git diff --cached`、未跟踪的 `correction.go`、`word_enrichment_test.go`、`WordCorrectionButton.vue` 和全部上游产物；复核期间实现智能体修复了下表问题，最终源码已重新检查。本审查仅新增本文件，没有修改实现代码。现有 AGENTS/profile 已存在，遵照「只写审查文档」的任务范围，没有运行会创建项目文件的引导脚本。技术栈：Go 1.24、net/http、database/sql/MySQL；Vue 3、Vite、Element Plus，前端无现有自动化测试框架。

## 第一步：是否符合上游产物

| 01-spec 验收标准 | 结论 | 依据 |
|---|---|---|
| 录入旧词/获取旧闪卡触发补齐，已含 derivatives（包括 []）不请求模型 | 达成，代码及本地替身层 | `SensesEnriched` 用两类关系字段非 nil 判断；`ensureWordEnrichment` 被录入、队列和评分调用；`TestFlashcardEnrichmentCacheAndNoRepeatedLookup`、`TestRepeatedInputReusesEnrichedCache` 覆盖旧词、缓存和 []。真实 MySQL 未运行。 |
| 两种复习页面分开展示派生词和形近词，保留点击查词 | 达成 | WordCard/FlashcardView 独立构造「派生词」「形近词」组并沿用 `splitWordRef`、查词入口；02-plan 两入口浏览器测试记录已通过。 |
| 两处纠错必填原因，发送原因和全套重新生成要求，成功即时显示新内容 | 达成 | WordCorrectionButton 双端校验 1–500 字；服务端传原因并要求全套；完整字段校验拒绝局部模型结果；成功只合并内容且同步已有闪卡单例。HTTP、两入口及跨页浏览器已通过。 |
| 无权限/查询中/模型失败不覆盖原内容，纠错不改变次数与排期 | 达成，本地验证层 | 401/404/400/409/422/503/500 测试，独立 `UpdateSenses` 不写复习字段；预约成功后重读恢复快照；模型/缓存/个人保存失败及旧快照测试均通过。浏览器失败保留内容、1/1、×7 和翻面。 |
| Go 测试、前端构建和局部 lint 通过；浏览器以本地测试数据走完整入口 | 达成 | 构建、race、vet、lint、空原因/成功/失败入口均已通过；最终旧卡页面确认派生词随轮询自动出现，翻面、1/1、×7 保留，已回填 02-plan。 |

- [x] 02-plan 的实现文件清单全部完成；本文件完成独立复核项，由主智能体回填计划勾选。
- [x] 没有未解释的计划外源码文件。新增路由/类型/存储方法/测试/共享纠错组件均在计划范围；归档页同步属于共享 WordCard 调用方适配。
- [x] 通用能力判定与业务层落点一致，没有单客户逻辑。
- [x] 02b-design 接口、JSON、预约/恢复、缓存局部成功语义、前端事件/状态/轮询设计已核对；复核修复已回补设计。

### 复核问题与闭环

| 问题 | 后果与修复 | 最终状态 |
|---|---|---|
| 原模型校验仅要求释义及两类关系字段 | 局部模型结果可能覆盖旧音标/例句。已增加 `validateLookupCompleteness`：所有字段存在且非 null，词性/释义/音标/例句/译文非空；HTTP 503 保留旧内容测试已补。 | 已修复，已回补 02b |
| 列表纠错未同步已加载闪卡单例 | 返回闪卡可能继续背错误内容。HomeView/ArchiveView 已同步 `updateContent`，不重置队列；跨页浏览器已通过。 | 已修复，已回补 02-plan/02b |
| 较早轮询可能覆盖成功纠错内容 | 已增加每词本地版本，响应返回时版本变化则丢弃旧结果；真实队列模块延迟响应验证通过。 | 已修复，已回补 02b |
| 纠错旧快照在预约前读取 | 另一纠错完成后，后到请求失败可能恢复成更旧内容。已改预约后重读；重读失败仅 ClearTranslating；回归测试通过。 | 已修复，已回补 02b |
| 全局缓存与个人保存非跨表事务 | 完整新全局缓存先保存；个人保存失败保留旧个人内容，返回 500。新缓存有效，未违反个人失败保留边界；已明确设计和测试断言该局部成功语义。 | 已说明，不作为阻塞 |

## 影响面复核

以下为独立执行的完整原始输出，未删节：

```text
Impact of changing "translateWord" — 5 affected symbols:

backend/internal/app/translate.go
  function    translateWord:19

backend/internal/app/correction.go
  method      handleCorrectWord:61

backend/internal/app/dictionary.go
  method      handleRetryDictionary:114

backend/internal/app/words.go
  method      translateAndSave:186
  method      spawnTranslation:116
```

```text
Impact of changing "ensureWordEnrichment" — 5 affected symbols:

backend/internal/app/correction.go
  method      ensureWordEnrichment:29

backend/internal/app/words.go
  method      tryIncrementExisting:270
  method      handleAddWord:27
  method      handleFlashcardQueue:474
  method      handleFlashcardReview:489
```

```text
Impact of changing "updateContent" — 6 affected symbols:

frontend/src/composables/useFlashcardQueue.js
  function    updateContent:88
  function    refreshContent:96

frontend/src/views/FlashcardView.vue
  component   FlashcardView:1
  file        FlashcardView.vue:1

frontend/src/views/ArchiveView.vue
  function    handleCorrected:18

frontend/src/views/HomeView.vue
  function    handleCorrected:200
```

- [x] 所有已识别调用方已核对。管理员词库重试沿用新模型字段校验；旧 JSON 纯解析保留兼容。
- [x] 与 spec 预估基本一致；新增 `handleCorrectWord` 属预期新增入口；跨页内容同步由单例状态引出，已补设计。

### 分层检查

- [x] 改动触及共享领域类型、App 存储接口、WordCard 和闪卡状态模块。理由：本功能面向项目全部用户，是通用查词/复习能力，追加 JSON 字段兼容旧数据；没有客户条件分支。profile 仍是模板路径，不能据此当作真实公共层声明，上方 impact 为实际判断依据。
- [x] 关键新增方法已说明做什么及为什么；correction.go、WordCorrectionButton 顶部含职责/调用流。原 words.go 关键后台链路已有注释；无必须补写的注释问题。

## 验证记录

| 02-plan 验证前置 | 实际结果 | 通过 |
|---|---|---|
| Go 测试 + race：旧数据/[]、模型请求、权限、预约、失败恢复、成功保存及复习状态 | 独立复跑 `go test -race ./...`：app/model/storage 通过（缓存命中）；主智能体最终 race、vet 通过。源码核对新增完整字段、旧快照、缓存局部成功测试。 | ✅ |
| 前端构建、局部 ESLint、diff 空白检查 | 主智能体最后 build/lint 通过；独立 `git diff --check` 无输出。未重复 lint 已能检查的风格问题。 | ✅ |
| 两入口派生独立显示、空原因阻止、全套成功更新 | 02-plan 的 ego-browser 本地构建/测试 API 实测通过；模型请求内容由 HTTP 替身测试核对。 | ✅ |
| 故障保留内容与进度 | 背单词与闪卡浏览器失败通过；闪卡 1/1、×7、翻面保留。 | ✅ |
| 旧闪卡补查后更新内容 | 后端旧卡补查与实际队列模块异步响应通过；最终独立快照的浏览器补验：开始 pending 且派生未显示，翻面后轮询解除状态，worked/worker 自动出现、形近 word 独立显示，flipped=true、1/1、×7 保留。主智能体实测结果已回填 02-plan。 | ✅ |
| 轮询与跨页恢复 | 实际队列模块延迟旧响应不覆盖新纠错；浏览器跨页回到闪卡显示新内容且进度保留。 | ✅ |

## 未验证项与剩余风险

| 未验证/风险项 | 为什么现在验不了 | 补验方式 + 负责人 + 时间 |
|---|---|---|
| 真实 MySQL 缓存 SQL/原子预约 | app fake 的 SaveSenses 实际为无条件 Replace，不能证明 SQL 条件行为；本次未接生产数据库。源码条件覆盖缺失和 JSON null，[] 不命中关系缺失条件；强制 Replace 为 upsert。 | 浩原授权的发布/联调执行者，在隔离 MySQL 建旧 JSON、null、[]、完整缓存四组样本，验证 Save/Replace 和两个连接并发预约的行数；发布前或首次联调时执行。 |
| 真实模型语义质量、线上页面 | 本次使用本地模型替身/测试 API，未访问真实模型和生产环境；服务端只能检查结构并去除模型明确给出的交叉，不能证明派生关系事实。 | 浩原授权的发布/联调执行者，用 work/worked、form/from、quite/quiet 等真实词核对全套字段及关系分类，线上两入口验收；下次授权发布时执行。 |

本地同词锁属于单 App 进程；未承诺多实例同词全局串行。个人预约由数据库保证同词条互斥；跨用户/跨实例缓存强制纠错先后顺序仍由实际写入决定。正常补全失败保留旧内容，未来再次录入/取队列可重试；永久存储故障只能记录失败并由现有恢复任务/运维处理。

## 测试与遗留事项

- [x] 已补 `backend/internal/model/models_test.go`、`backend/internal/app/word_enrichment_test.go`、`backend/internal/app/routes_test.go`。存储 JSON 兼容、完整纠错和并发恢复适合自动测试；前端本次用浏览器与真实模块受控异步验证。
- 不提交、不推送、不发布、不批量回写历史词库；这些均是 02-plan 明确的本次范围。
- 真实 MySQL/真实模型保持分层证据，不因构建或测试替身通过而自动宣称生产验收。

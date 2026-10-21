<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-21 07:46 / 结束 2026-10-21 07:54 / 规范版本 @sw/dev-skills@1.1.2 -->
# 派生词展示与单词纠错

## 需求陈述
录入和闪卡复习补齐旧单词的派生词，派生词与形近词分开展示；背单词和闪卡提供必填原因的纠错，重新查询所有内容。

## 定制 or 通用
通用能力，面向本个人项目所有用户。无客户定制模块，沿用项目前后端业务层。

## 影响面
实际 `codegraph impact translateWord` 输出：
```
Impact of changing "translateWord" — 4 affected symbols:
backend/internal/app/translate.go function translateWord:19
backend/internal/app/dictionary.go method handleRetryDictionary:114
backend/internal/app/words.go method translateAndSave:186, method spawnTranslation:116
```
已确认：领域 Sense、查询服务、个人/全局词库、闪卡和单词卡片受影响。推断：新增字段影响所有返回 senses 的接口，兼容追加字段。

## 依赖服务现状
已确认：lookupDeepSeek(ctx, word, cfg) 调用 /chat/completions，20 秒超时；需扩展原因参数。WordStore.UpdateSenses 只写 senses/translating，复习排期及重要释义独立保存。DictionaryStore.LookupSenses/SaveSenses 可复用缓存。

## 核心语义与数据模型
- [已确认] word.id 是个人词条主键；word_key 为小写精确词键，全局缓存以其关联。
- [已确认] Sense 当前只有 lookalikes，没有 derivatives；内容存于 JSON，不需要数据库字段迁移。
- [设计] derivatives 为字符串列表，如 worked（work 的过去式/过去分词：工作过）、worker（名词：工人）；[] 表示已查询且无派生词，null/缺失表示旧数据待补。
- [设计] lookalikes 为多/少/替换一个字母或相邻字母交换的独立易混词，不能包含上述派生变化。
- [设计] 录入和闪卡队列遇到有正常释义且缺字段的旧词，优先复用新缓存，再后台查询；查询中不重复提交。
- [设计] 纠错同步调用模型，原因随请求送入模型，成功更新个人词条及全局缓存；其他用户已有个人词条不批量重写。失败保留原内容，复习次数、归档、SRS、重要释义不变。

## 设计要点
采用显式字段状态而非数组长度判断是否已补齐，避免无派生词时反复查。纠错强制绕过缓存，提交期间禁用同词复习动作，使用原子查询状态预约防止并发。闪卡轮询只更新内容，不重置进度。正常查询同词串行并复查缓存。

## 可复用资产
复用 Sense JSON、后台查询限流、词库缓存、GET /api/words/translating?ids=、Element Plus 对话框和现有 Go 测试；无建表，sql/upgrade 不适用。

## 边界行为清单
- [x] 空数据：derivatives=[] 已完成；旧字段 null 待补；无正常释义按原录入/重试处理。
- [x] 异常：空/超长原因 400；非本人词条 404；查询中 409；模型失败 503/拼写错误 422，保留旧内容。
- [x] 历史：JSON 增加字段，无数据库迁移，不批量请求旧库。
- [x] 测试替身：扩展 Go fake 覆盖预约/保存，不接生产数据库。
- [x] 契约：新增 POST /api/words/{id}/correct，返回更新词条；senses 新增 derivatives。
- [x] 权限：需登录，只能纠正本人词条；不依赖管理员身份。
- [x] 并发：DB 原子 translating=0 条件预约，同词模型调用串行；取消/失败释放状态。

## 验收标准
- [x] 录入旧词/获取旧闪卡触发补齐，已含 derivatives（包括 []）不请求模型。
- [x] 两种复习页面分开展示派生词和形近词，保留点击查词。
- [x] 两处纠错必填原因，发送原因和全套重新生成要求，成功即时显示新内容。
- [x] 无权限/查询中/模型失败不覆盖原内容，纠错不改变次数与排期。
- [x] Go 测试、前端构建和局部 lint 通过；浏览器以本地测试数据走完整入口。

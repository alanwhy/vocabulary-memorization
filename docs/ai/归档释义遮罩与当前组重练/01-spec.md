---
task_id: 20261120-0741-archive-group-retry
status: awaiting_user
current_stage: impl
stage_status: ready
execution_mode: auto_to_impl
updated_at: 2026-11-20T07:46:00+08:00
review_base: c3b750a54818e7b0a7f1aed4d7cb9eab7c0c58ee
---

# 归档释义遮罩与当前组重练

执行模式：用户本轮选择“自动执行至实现（推荐）”。按用户项目指令减少独立复核，由用户手动验收；不含发布或飞书归档授权。

阶段记录：模型 GPT-6（具体运行型号未提供）；spec/plan 2026-11-20 07:40–07:41、impl 07:41–07:46 +08:00；规范 @sw/dev-skills@2.0.0。实现、自动测试与本地隔离页面验证已完成，真实账号业务验收待用户执行，见 03。

## 需求与适用范围

1. 已归档单词的中文释义默认模糊，点击后正常查看。适用于归档页，以及首页仍显示的已归档词；未归档词维持现有展示。
2. 闪卡一组完成后新增“当前组再来一次”，只复习本轮评分为“不认识”（again）和“模糊”（fuzzy）的词。支持连续重练，每轮以刚完成一轮的评分筛选；“再来一组”继续加载服务器到期队列。

## 现状与设计

- 已确认：ArchiveView 经 WordList 以 archived 模式渲染 WordCard；WordCard 也供 HomeView 使用。主释义点击原本用于标记重要释义，首次揭示需要拦截该动作。
- 已确认：useFlashcardQueue 是模块级单例，groupResults 在评分请求成功后追加 `{ word, rating }`，loadQueue 成功后清空。可直接筛选本轮结果构建重练队列，不请求到期接口。
- 设计取舍：单卡一次揭示该卡中文释义（主释义、例句译文、相关词释义），英文与发音保持可用；通过可聚焦按钮支持键盘揭示。单卡卸载重挂载、单词身份/归档状态/模式改变时恢复默认遮罩。
- 设计取舍：重练照常提交已有评分接口并累计复习次数；不改变后端 SRS、归档和权限规则。完成页统计当前轮；累计数量按成功评分次数计。

## 核心语义与流转

- 关键 ID：服务器单词 `id`，重练直接保留本轮结果里的单词对象及原顺序，排除 good 和其它值。
- 存储：遮罩仅组件内状态；重练仅本次应用会话内队列。评分继续 POST `/api/flashcards/review`；重练入口自身不发请求，不新增表、接口或本地持久化。
- 多入口：归档标记 `word.archived` 或 archived 展示模式均触发遮罩；队列调用方的 updateContent 接口不变。

## 边界行为

- 无释义、查询中、错误提示不遮罩；无可重练词时不出现重练按钮，方法调用也无副作用。
- 评分失败保留当前词，不记结果、不推进；下一组加载失败保留完成页和重练能力。
- 只在本轮完成后允许重练，加载/提交期间和重复点击不能重置进行中的队列；切换页面保留重练进度。
- 历史字段和权限保持原契约；无迁移。mock 仅用于验证，不进入产品运行逻辑。
- 项目声明仍为未确认模板，不采用其中示例路径；按实际调用关系兜底，本次是本应用通用业务需求，无客户定制或跨项目公共工具改动。
- bootstrap 返回 legacy-needs-confirmation，未迁移或覆盖 AGENTS.md。起始工作区仅有未跟踪 frontend/pnpm-lock.yaml，保留不动。

## 验收标准

- [ ] 归档单词中文默认模糊，点击文字或查看按钮后揭示；首次点击不标记重要释义。
- [ ] 未归档词正常展示；英文、音标、发音仍可使用，其他归档词不会被一起揭示。
- [ ] 混合评分组完成后，重练只含 again/fuzzy，顺序不变；再次重练排除上一轮已记住的词。
- [ ] 全认识、空队列、未完成、加载或提交中均不能启动重练。
- [ ] 下一组、失败重试、切页保留进度与累计数量仍正常；完成构建与适用行为验证。

## 依赖与影响面证据

`apiPost(url, body)` 和 `apiGet(url)` 满足现有评分/下一组请求；不改公共请求层。后端 good 直接归档，again/fuzzy 更新排期，重练复用该契约。

实际 CodeGraph 原始 impact 输出（2026-11-20）：

```text
Impact of changing "useFlashcardQueue" — 8 affected symbols:

frontend/src/composables/useFlashcardQueue.js
  function    useFlashcardQueue:114

frontend/src/views/ArchiveView.vue
  function    handleCorrected:18
  component   ArchiveView:1
  file        ArchiveView.vue:1

frontend/src/views/HomeView.vue
  function    handleCorrected:201
  component   HomeView:1
  file        HomeView.vue:1

frontend/src/views/FlashcardView.vue
  file        FlashcardView.vue:1

Impact of changing "WordCard" — 7 affected symbols:

frontend/src/components/word/WordCard.vue
  component   WordCard:1

frontend/src/components/word/WordList.vue
  component   WordList:1
  file        WordList.vue:1

frontend/src/views/ArchiveView.vue
  component   ArchiveView:1
  file        ArchiveView.vue:1

frontend/src/views/HomeView.vue
  component   HomeView:1
  file        HomeView.vue:1
```

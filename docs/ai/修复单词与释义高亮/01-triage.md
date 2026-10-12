<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-12 / 结束 2026-10-12 / 规范版本 @sw/dev-skills@1.1.2 -->

# 修复例句词形与中文释义分号高亮

## 复现与根因

1. 在单词例句中使用 `study` 的 `studied/studying`、`make` 的 `making` 或 `run` 的 `running` 等形式。
2. 在闪卡或单词列表里点击中文释义的第二个义项。

- **期望**：当前单词的常见词形沿用该词的次数颜色；中文高亮只覆盖义项文字，分号保持普通样式。
- **实际（用户报告，源码确认）**：`tokenizeExample` 只把例句 token 转小写并精确查词，闪卡还要求 token 等于当前 `word_key`，因此词形变化不命中。两个释义模板把前导分号放在 `.gloss` 内，分号随义项一起变红、加粗、斜体。
- **检查基线**：本地 `main`，HEAD `119461354aead3d33de4e76b0f22d7c53ff9d9cf`，开始时工作区干净。未启动页面进行浏览器复现。

## 定制 or 通用

个人项目的通用缺陷修复，覆盖所有用户的例句展示与重要释义展示；无客户定制。共享工具变更有意覆盖单词列表、闪卡和查词弹层，但闪卡继续只高亮当前词。

## 核心语义与边界

- 例句保留原文与点击查词内容；高亮次数来自命中的词库原形。当前单词的词形优先，其他单词仍按原有全库规则识别。
- 支持常见 `s/es/ies/ied/ed/ing`、去 `e`、双写、`ie→ying` 和有限的常见不规则形式。候选词根经过正向词形校验，避免 `hop→hoping`、`be→bed` 等误判；不采用前缀匹配或任意派生后缀。
- 中文释义按全角或半角分号拆分；分隔符不参与点击标记，不写入 `important_glosses`。已有全角分号数据与已保存标记保持匹配。
- 空例句/空释义与词库未加载时维持不高亮降级。无接口、数据库、权限、请求幂等或后端持久化流程变更。

## 影响面

`codegraph impact tokenizeExample` 原始输出：

```text
Impact of changing "tokenizeExample" — 9 affected symbols:

frontend/src/utils/highlight.js
  function    tokenizeExample:3

frontend/src/components/word/WordCard.vue
  function    exampleTokens:70
  file        WordCard.vue:1

frontend/src/components/word/WordLookupTooltip.vue
  function    exampleTokens:15
  file        WordLookupTooltip.vue:1

frontend/src/views/FlashcardView.vue
  function    exampleTokens:75
  file        FlashcardView.vue:1

frontend/src/components/word/WordList.vue
  file        WordList.vue:1

frontend/src/App.vue
  file        App.vue:1
```

`codegraph impact splitGlosses` 原始输出：

```text
Impact of changing "splitGlosses" — 5 affected symbols:

frontend/src/utils/gloss.js
  function    splitGlosses:3

frontend/src/components/word/WordCard.vue
  function    glossItems:59
  file        WordCard.vue:1

frontend/src/views/FlashcardView.vue
  file        FlashcardView.vue:1

frontend/src/components/word/WordList.vue
  file        WordList.vue:1
```

## 修复思路与可复用资产

复用 `tokenizeExample` 的分词与 count 返回结构，在共享工具补充词形匹配与可选当前词优先；三个调用方传各自当前词，闪卡保留限定 lookup。复用 `splitGlosses` 和现有重要义项状态，只将分号渲染成 `.gloss` 外的独立标签。

## 验收标准

- `study/studied/studying`、`make/making/made`、`run/running/ran`、`stop/stopped/stopping` 等形式可命中当前词，使用当前词次数。
- `play/player`、`hop/hoping`、`be/bed` 不因前缀或机械删后缀误命中。
- 闪卡正反面只高亮当前词的有效词形；列表和查词弹层保留全库高亮，原文与查词点击不变。
- 闪卡与列表重要释义不包含分号；半角分号也可拆分，切换义项仍只提交义项文字。
- 前端生产构建成功。

## 复现测试

- [x] 前端无测试基建：限定扩展名检索无测试文件，`package.json` 无测试框架/测试命令。按当次默认要求执行可编译性验证；浏览器操作路径列入计划，未执行项在复核中注明。

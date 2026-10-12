<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-12 / 结束 2026-10-12 / 规范版本 @sw/dev-skills@1.1.2 -->

# 前端详细设计

## 例句词形

共享工具增加可选当前词参数 `tokenizeExample(text, lookup, preferredWord = '')`，不改变 token 的 `{ text, count, isWord }` 结构。当前词有效词形且词库有次数时优先使用当前词次数；否则精确匹配原 token，再对候选词根进行正向词形校验与词库查找。

规则覆盖常见三单/复数、过去式、进行时；常见不规则动词使用有限显式表。短单音节 CVC 词严格要求双写，避免 `hop` 误命中 `hoping`；长词保留普通词尾与双写候选，避免 `visit/open` 受重音启发式误伤。词形规则不负责判定例句语义，也不泛化 `ly/ness/er` 等派生后缀。

三个调用方分别传 `current.word_key`、`props.word.word_key`、`state.wordKey`。闪卡仍用只允许当前词的 lookup，列表/弹层仍传 `vocab.lookup`。不改原文或查词事件。

## 中文义项

`splitGlosses` 按 `/[；;]/` 拆分，返回 trim 且过滤空项的字符串数组。两个可点击模板用 `<template v-for>` 包装每个义项，独立普通 `<span>` 放置分号，`.gloss` 只包含义项文本、样式绑定与点击事件。分号显示沿用全角样式。

无新增组件或状态，重要标记的集合与原有 API 请求结构不变。

<!-- 阶段记录：模型 GPT-6（Codex，具体子型号未提供） / 计划 2026-10-30 07:51 CST / 实现结束 2026-10-30 07:52 CST / 规范 @sw/dev-skills@1.2.0 -->

# 实现计划

## 背景

feature：按 [01-spec.md](01-spec.md) 增加闪卡键盘操作，保持与按钮一致。

## 验证方式

### 验证前置

| 命令/操作路径 | 预期结果 |
|---|---|
| 闪卡页 → 空格 → 再次空格 → V → 1/2/3 | 来回翻面、不滚动，朗读当前词，评分成功只进入下一张且组内结果正确；待用户手动验收 |
| 翻面/发音按钮获得焦点后按空格 | 只翻面，不额外点击焦点按钮；待用户手动验收 |
| 长按快捷键、打开纠错/查词、输入反馈、提交中、内容补全中、切出页面 | 符合 01 边界规则；待用户手动验收 |
| `cd frontend && npm run build` | 前端生产构建成功 |
| `cd frontend && npx eslint src/views/FlashcardView.vue && npx oxlint src/views/FlashcardView.vue` | 本次文件检查通过，不自动修复其它文件 |
| `git diff --check` | 无空白错误 |

### 受影响的测试

`codegraph affected frontend/src/views/FlashcardView.vue` 返回 `ℹ No test files affected by the changed files.`；前端测试文件数为 0，且没有测试命令。页面入口改动采用验证前置，不新增测试基建。按用户要求不启动独立复核，保留手动验收项。

## 改动方案

- [x] `frontend/src/views/FlashcardView.vue` —— 为评分配置增加数字键映射，增加带状态、输入及弹窗保护的键盘处理，挂载注册/卸载移除监听 —— 文件检查、构建及按键处理模拟检查通过；实际手动操作待用户验收。
- [x] `frontend/src/views/FlashcardView.vue` —— 增加快捷键说明与翻面提示 —— 构建通过；待用户目视验收。
- [x] `docs/ai/闪卡键盘快捷键/03-review.md` —— 记录实际检查及待用户验收，不独立复核。

### 本次不做

不改复习规则、发音来源、公共工具及其它页面。实现完成后，用户追加授权使用 quick-release 发布 v1.23.0，发布边界见 03。

## 关键文件

| 文件 | 改动点 | 依赖摘要 |
|---|---|---|
| `frontend/src/views/FlashcardView.vue` | 键盘事件与提示 | 使用已有 `flip`、`rate`、`speakWord` 和队列状态；其它页面不受影响 |

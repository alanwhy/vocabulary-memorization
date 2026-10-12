<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-12 / 结束 2026-10-12 / 规范版本 @sw/dev-skills@1.1.2 -->

# 实现计划

## 背景

bugfix：统一补充例句的常见词形匹配，并将中文释义分号与重要标记分开渲染。根因与影响面见 `01-triage.md`。

## 验证方式（实现前确定）

- 执行 `cd frontend && npm run build`，预期生产构建成功，所有修改的 Vue 模板可编译。
- 端到端补验路径：进入闪卡与单词列表，检查含 `studied/studying`、`making`、`running` 的例句，当前词词形应使用该词颜色；闪卡其他单词仍不高亮。点击第二个中文义项，只有文字高亮，分号保持原样；再次点击可取消，重新加载后标记保持。
- 按 Alan 的当次默认验证要求，不自行启动 dev server 或浏览器；上述页面路径记录为未执行补验项，源码复核逐项核对逻辑。
- `codegraph affected frontend/src/utils/highlight.js` 与 `codegraph affected frontend/src/utils/gloss.js` 均返回 `ℹ No test files affected by the changed files.`。前端无测试基建，不新增测试框架。

## 改动清单

- [x] `frontend/src/utils/highlight.js`：增加有限词形生成/候选还原与正向校验，`tokenizeExample` 支持可选当前词优先；复核原文/count/边界，执行构建。
- [x] `frontend/src/utils/gloss.js`：支持全角和半角分号，保留 trim/过滤空项；复核既有义项键兼容，执行构建。
- [x] `frontend/src/views/FlashcardView.vue`：传当前词给 tokenizer，保留仅当前词 lookup；两个词面保持共用函数，分号移至义项标签外；源码核对并构建。
- [x] `frontend/src/components/word/WordCard.vue`：传当前词供优先匹配，分号移至独立标签；源码核对并构建。
- [x] `frontend/src/components/word/WordLookupTooltip.vue`：传正在查询的词作为优先词，保持全库高亮；源码核对并构建。
- [x] 执行前端构建并记录真实结果，启动独立 sw-review 子智能体生成 `03-review.md`（有条件通过，页面与持久化未验证）。

## 关键文件与设计

- `tokenizeExample` 被闪卡、WordCard、WordLookupTooltip 的 `exampleTokens` 调用；原有两参数调用保持兼容，新增可选第三参数 `preferredWord`。
- `splitGlosses` 被列表 `glossItems` 和闪卡 `senseGlosses` 调用，继续返回字符串数组。
- 前端渲染细节见 `02b-design.md`；无接口、表结构或后端改动。
- 当前词优先后，全库场景仍先匹配例句原词，再尝试有效词根；闪卡的查找函数继续只接受当前词。

## 范围

不调整重要释义 API、乐观更新、颜色档位、例句查词内容，不增加全面词法分析或任意同词根词义匹配。原实现阶段不包含上线；后续发布按下方用户授权执行。

## 已执行验证

2026-10-12 最终代码已执行 `cd frontend && npm run build`，退出码 0；2194 个模块完成转换，482ms 完成生产构建。仅有既有 bundle 体积告警。`git diff --check` 通过。页面操作与服务端持久化未验证。

## 后续发布授权

2026-10-12 Alan 显式调用本仓库 `quick-release` 技能，授权发布本次修复。按纯缺陷修复由 `1.20.0` 升补丁版本至 `1.20.1`，同步 package/lock 与 CHANGELOG。

发布前实时核对：`main` 与 `origin/main` 均为 `119461354aead3d33de4e76b0f22d7c53ff9d9cf`，无待整合提交；工作区仅本次 5 个前端文件与 4 份流程文档。生产 SSH 可达，已有 MySQL/音频卷均存在。无数据库结构或数据迁移，无需迁移备份。

后续按 `release: v1.20.1` 提交并推送 `origin main`，推送成功后使用文档规定的 rsync 排除项同步，再在生产服务器执行 Compose 源码构建与启动。发布阶段依技能不运行本机 Docker、编译、测试、线上冒烟或功能验收；上述构建记录属于先前实现阶段。

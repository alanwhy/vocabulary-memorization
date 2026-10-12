<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-12 07:39 / 结束 2026-10-12 07:43 / 规范版本 @sw/dev-skills@1.1.2 -->

# 自审与验收

结论：**有条件通过**。最终改动无未闭环的关键问题，生产构建成功；计划中的页面操作与持久化补验未执行。

## 前置检查

- 独立复核依据：`01-triage.md`、`02-plan.md`、`02b-design.md`，以及完整 `git diff` 与 `git diff --cached`；暂存 diff 为空。
- 复核基线：`main`，HEAD `119461354aead3d33de4e76b0f22d7c53ff9d9cf`。业务源码改动限于计划列出的 5 个文件，另有本次流程产物。
- 已运行项目引导脚本；`.claude/ai-profile.md` 与根 `AGENTS.md` 均已存在，脚本跳过创建，没有修改它们。
- 技术栈：Vue 3、Vite、JavaScript；本次没有后端修改。限定扩展名检索未发现前端测试文件，`frontend/package.json` 没有测试框架或测试命令，按无测试基建处理。
- 理解代码时优先使用 `codegraph_explore`；其裁剪未覆盖的 Vue 模板与样式片段使用定点读取补齐。没有启动 dev server、浏览器或额外运行行为用例。

## 第一步：是否符合上游产物

### 验收标准逐条核对

| 验收标准 | 结论 | 依据 |
|---|---|---|
| `study/studied/studying`、`make/making/made`、`run/running/ran`、`stop/stopped/stopping` 等形式沿用当前词次数 | 达成（源码核对） | `wordForms` 覆盖辅音加 y、去 e、短 CVC 双写和显式不规则形式；`tokenizeExample` 先查当前词有效词形，再精确查原 token，最后查经正向校验的词根。实际页面颜色未验证。 |
| `play/player`、`hop/hoping`、`be/bed` 不因前缀或机械删词尾误命中 | 达成（源码核对） | 没有前缀匹配或 `er` 派生规则；`hop` 生成 `hopping` 而非 `hoping`；`be` 只接受短词原文与显式词形，不生成 `bed`。复核中发现的 `sing/singe` 边界已修复。 |
| 闪卡正反面只高亮当前词有效词形；列表与弹层保留全库高亮，原文与查词点击不变 | 达成（源码核对） | 闪卡两个词面共用 `exampleTokens`，其 lookup 仍只接受 `current.word_key`；WordCard 与弹层继续使用 `vocab.lookup`。返回值仍为 `{ text, count, isWord }`，显示和查词继续使用原 token 的 `text`。 |
| 闪卡与列表重要释义不含分号；兼容半角分号，切换时仅提交义项文字 | 达成（源码核对） | `splitGlosses` 使用 `/[；;]/`、trim、过滤空项；两处模板把分号放在 `.gloss` 外的普通兄弟 span。样式绑定和 `toggleGloss(g)` 只在义项 span 上，分号不在标记事件和提交值内。实际点击、取消和重新加载后的状态未验证。 |
| 前端生产构建成功 | 达成（实际执行） | 主智能体在最终 `singe` 修正后执行 `npm run build`：退出码 0，2194 个模块完成转换，`built in 482ms`；仅有既有 chunk size warning。 |

### 计划、详细设计与范围

- [x] 5 个业务文件均已完成计划中的修改，没有计划外业务源码改动。
- [x] `02b-design.md` 的可选第三参数、token 结构、查找优先顺序、调用方范围和独立分号标签均与实现一致。
- [x] 「通用缺陷修复」的判定与共享工具及三个展示入口的落点一致，无客户特有判断。
- [x] 没有修改接口、表结构、重要释义保存流程、颜色规则或发布配置。
- [x] 构建与独立复核已实际完成；主智能体在本报告落盘后同步计划最后一项。

| 偏差 | 原因与处理 |
|---|---|
| 无设计或范围偏差 | 复核中补充不规则词与 `singe` 拼写例外，属于上游约定的「有限显式不规则形式和正向校验」，没有增加新的匹配后缀或改变接口。 |

### 复核发现与闭环

| 发现 | 后果 | 最终处理 |
|---|---|---|
| 早期有限表缺少 `sing` | 规则会把 `singed`（`singe` 的过去式）误配到当前词 `sing` | 主智能体补充 `sing → sang/sung` 等常见不规则形式，已重新核对最终源码；不再机械生成 `singed`。 |
| 早期去 e 规则应用于 `singe` | 会把 `singing` 误配到 `singe`，并漏掉 `singeing` | 主智能体增加 `singe` 进行时保留 e 的显式分支及原因注释；已重新核对最终源码与最终构建结果。 |

## 第二步：影响面复核

最终源码上的 `codegraph impact tokenizeExample` 原始输出（未省略）：

```text

Impact of changing "tokenizeExample" — 9 affected symbols:

frontend/src/utils/highlight.js
  function    tokenizeExample:119

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

最终源码上的 `codegraph impact splitGlosses` 原始输出（未省略）：

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

- [x] 三个 tokenizer 调用方均已确认；两个义项展示入口均已确认，没有漏改的直接调用方。
- [x] 与 `01-triage.md` 预估的符号及文件范围一致，仅源码增加后行号发生变化。WordList 与 App 属于原有间接使用范围，没有新增入口。

## 第三步：分层检查

- [x] 改动触及共享工具层，理由：这是所有用户都需要的通用词形与释义显示修复，三个例句入口复用一致规则；闪卡限制通过原有 lookup 注入保留，没有把闪卡特有范围下沉为共享工具的业务判断。
- `.claude/ai-profile.md` 仍是示例模板，没有明确声明本仓库真实公共层路径。本次结合实际 `frontend/src/utils/` 路径和上方 impact 原文作启发式判定，不将示例路径误当成已确认声明。
- [x] 关键方法有「做什么 + 为什么」注释，说明正向校验、短 CVC 双写、当前词优先与闪卡范围限制。
- [x] `highlight.js` 顶部说明三个例句入口共用工具、词形只用于次数、原文继续显示和查词；`gloss.js` 顶部说明后端分隔来源及拆分目的。

## 第四步：验证记录

| 验证项 | 实际结果 | 通过 |
|---|---|---|
| 前端生产构建 | 主智能体在最终修正后实际执行：退出码 0，2194 modules，482ms。既有 chunk size warning 保留。独立复核没有重复运行构建。 | ✅ |
| 修改文件与当前基线核对 | 独立获取完整未暂存 diff 和暂存 diff；5 个业务文件符合计划，分支及 HEAD 未变。主智能体记录 `git diff --check` 通过。 | ✅ |
| 无测试基建确认 | 限定测试扩展名的前端检索无结果；package.json 无测试框架/命令；没有新增框架或运行假测试命令。 | ✅ |
| 词形高亮、范围、原文与点击内容的源码复核 | 独立核对当前词优先、精确 token 优先于还原、正向校验、原 token 保留及闪卡仅当前词 lookup。 | ✅ |
| 分号边界、半角拆分与保存参数的源码复核 | 两个模板的分号均在 `.gloss` 与其点击事件外，事件仅传 g；现有集合与保存接口不变。 | ✅ |
| 闪卡、列表、查词弹层的实际页面词形和颜色检查 | 未执行。按 Alan 当次默认验证要求，不自行启动 dev server 或浏览器。 | 未执行 |
| 重要义项实际点击、取消与重新加载后的持久化检查 | 未执行。仅确认源码与接口调用未变，未发起业务请求。 | 未执行 |

## 未验证项与剩余风险

| 未验证或风险项 | 为什么现在未验证 | 补验方式、负责人、时间 |
|---|---|---|
| 三个入口的词形颜色；闪卡其他词不高亮；原文及点击查词内容 | 本次仅授权默认可编译性验证，未启动页面或行为用例 | Alan 或后续获明确页面验证授权的主智能体：进入闪卡正反面、列表、弹层，检查 `studied/studying`、`making/made`、`running/ran` 及同时存在其他已录入词的例句；补验时间为下次明确页面验收时。 |
| 分号普通样式、半角拆分、重要义项取消与重载后保持 | 未进行浏览器操作与服务端保存/读取 | Alan 或后续获明确页面验证授权的主智能体：点击第二个义项，核对分号样式，取消后重载；再标记并重载确认保存，覆盖全角/半角分号；补验时间为下次明确页面验收时。 |
| 有限英语词形规则无法判断语境、所有重音或全面不规则词 | 上游明确限定常见词形与有限表；源码规则不能代替全面词法分析 | Alan 提供新误判或漏判例句时，由后续主智能体确认具体词形，再补有限规则并按当次验证要求复核；时间为新问题被报告时。当前不扩大到派生词或全面词法分析。 |

## 测试与遗留事项

- [x] 前端无测试基建，依照流程使用预先声明的验证方式；本次没有要求新增测试框架。
- 实现复核完成时尚未发布上线。页面与持久化补验的边界、补验方式、负责人及时间已逐条列在上方；没有另行遗留的实现任务。后续 Alan 于 2026-10-12 显式授权 quick-release 发布 v1.20.1，发布范围及前置核对见 `02-plan.md`，该授权不改变本报告的未验证项。

## 本次值得沉淀

| 值得沉淀的点 | 以后该怎么做 |
|---|---|
| 正向校验仍可能机械生成另一词的合法拼写，如 `sing/singe` | 对发现的歧义补显式不规则或拼写例外，再核对当前词优先是否会放大误判；不能只检查目标正向例子。 |

## 结论

- [x] **有条件通过**：无未闭环关键问题；上游产物、范围与分层一致；构建已实际通过。计划中的页面操作和持久化补验未执行，不能把源码推导或构建成功当作页面验收证据。补验后回填本报告验证记录。

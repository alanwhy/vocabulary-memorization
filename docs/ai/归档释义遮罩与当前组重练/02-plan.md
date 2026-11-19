# 实现计划

引用 [01-spec.md](01-spec.md)，状态与授权以该文件为准。阶段记录：GPT-6；2026-11-20 07:41 +08:00；@sw/dev-skills@2.0.0。

## 验证前置

| 命令/操作路径 | 预期结果 |
|---|---|
| Node 内置测试：加载混合队列→评分完成→当前组重练→再次评分→重练 | 仅保留 again/fuzzy，保持原顺序，每轮结果清空，累计计数保留，全认识后无可重练词 |
| Node 测试：加载/提交/未完成/空组/请求失败/重复点击 | 状态不会被意外重置，失败可重试，下一组行为不变 |
| ego-lite 本地隔离数据页面：归档词→点击中文，另一词→键盘查看；未归档词对照 | 默认 blur、单卡揭示、主释义/例句译文/相关词释义同步揭示，首次不发标记请求 |
| ego-lite 本地隔离数据页面：完成闪卡→重练→完成→再来一组 | 按钮接线与队列状态一致，空/全认识无重练入口 |
| `npm run build`；指定改动文件 eslint；`git diff --check` | 构建、静态检查与 diff 格式通过 |

## 测试入口与职责

现有前端 `audioPlayback.test.js` 使用 node:test；package.json 无 test 脚本。Vue 已安装，可用 Node registerHooks 解析现有 `@/api/client`，通过受控 fetch 调用真实队列与客户端实现，不安装依赖。队列是一般业务规则，补充有意义的多轮/并发/失败行为测试；遮罩由本地浏览器操作验证。CodeGraph 未发现该队列现有测试。

## 改动清单

- [x] `frontend/src/components/word/WordCard.vue`：归档判定、单卡揭示状态、点击拦截、键盘按钮及中文遮罩；浏览器验证。
- [x] `frontend/src/composables/useFlashcardQueue.js`：可重练数量与启动动作、完成态和互斥守卫；Node 行为测试。
- [x] `frontend/src/views/FlashcardView.vue`：完成页按钮、数量提示与响应式布局；浏览器验证。
- [x] `frontend/src/composables/useFlashcardQueue.test.js`：多轮筛选、计数、失败与并发防护；实际执行。
- [x] 本目录 `03-review.md`：记录实际验证、未验收项，按用户要求不启动独立复核。

实现按详细设计 [02b-design.md](02b-design.md)。不改后端、接口、数据库、鉴权，不发布，不处理已有未跟踪锁文件。遮罩并非数据安全机制，只用于回忆练习；重练成功评分照常更新服务器排期。

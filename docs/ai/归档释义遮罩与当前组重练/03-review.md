# 实现与验证记录

状态与授权引用 [01-spec.md](01-spec.md)。模型 GPT-6；2026-11-20 07:41–07:46 +08:00；规范 @sw/dev-skills@2.0.0。

按浩原的项目指令减少独立复核，本次未启动子智能体或独立 sw-review。以下记录主智能体实际执行的验证，不代表用户业务验收通过。

## 实际改动

- WordCard 对 archived 模式或已归档单词启用单卡中文遮罩，覆盖主释义、例句译文和相关词释义。文字点击或原生按钮键盘操作可揭示；捕获阶段停止首次点击传播，避免误触重要释义。单词 ID、归档状态、模式变化会复位。
- 队列新增 retryCount 和 restartGroup。只在本轮完成且不忙碌、有候选时按原顺序重练 again/fuzzy；不重新拉取队列。每轮重置结果、索引、翻面和错误，累计成功评分次数继续保留。
- 闪卡完成页新增“当前组再来一次（数量）”和说明，保留“再来一组”。累计文案改为复习次数，适应多轮重练。
- 新增六个 node:test 行为测试，使用真实 Vue 队列与客户端、受控 fetch，不安装依赖、不改生产注入逻辑。

## 已执行验证

| 验证 | 实际结果 |
|---|---|
| `cd frontend && node --test src/composables/useFlashcardQueue.test.js src/utils/audioPlayback.test.js` | 13/13 通过，其中本任务 6 项。覆盖连续重练筛选和顺序、累计计数、重复调用、未完成守卫、切页共享进度及内容更新、提交中/提交失败、加载中/加载失败、空队列/全认识、下一组、历史 hard 排除 |
| `cd frontend && ./node_modules/.bin/eslint src/components/word/WordCard.vue src/views/FlashcardView.vue src/composables/useFlashcardQueue.js src/composables/useFlashcardQueue.test.js` | 退出码 0，无错误 |
| `cd frontend && npm run build` | 退出码 0，2215 模块构建成功；提示部分产物超过 500 kB，非构建失败 |
| `git diff --check` | 退出码 0 |
| ego-lite 真实组件、本地隔离数据页面 | 已完成下述交互验证；不连接真实服务，不修改真实单词 |

### ego-lite 页面结果

临时 Vite 页面直接加载生产 WordCard、FlashcardView 和现有 CSS；mock 仅位于临时页面的 fetch/Audio，未写入产品。task space 17 已关闭，临时服务已停止。

1. 两张归档词与首页已归档词计算样式均为 `blur(5px)`；未归档词为 `none`。中文主释义、例句译文、相关词释义都被遮罩，英文例句不模糊。已查看截图 `/tmp/vocab-archive-blur.png`。
2. 点击第一张卡的义项只揭示该卡，标记事件仍为 0；另一张归档词用 Enter 揭示成功。未归档词点击仍产生重要释义事件。复用组件变更单词 ID 后遮罩恢复。
3. 首轮三词评分 again/fuzzy/good，完成页显示重练 2 词。点击重练按 apple→bridge 顺序进入两词队列；切出再返回保留 2/2 进度。
4. 第二轮评分 good/fuzzy，再次重练只剩 bridge；记住后不再出现重练按钮。完整评分 ID 为 `[1,2,3,1,2,2]`，重练过程中队列接口仅调用 1 次。已查看截图 `/tmp/vocab-retry-result.png`。
5. 点击“再来一组”才第二次请求队列，空结果显示“今日复习完成”。

## 工作区与范围

开发交付时，本任务修改三个前端生产文件，新增一个测试文件和本目录四份文档。起始未跟踪的 `frontend/pnpm-lock.yaml` 保留。未改后端、接口、数据库、鉴权或公共请求工具；该阶段未提交、推送、部署或归档。

用户本轮已选择“暂不归档（推荐）”；不访问飞书账号、不上传文档。

## 后续发布授权

2026-11-20 用户显式调用 quick-release，并选择生产环境、无任务号。版本按新增兼容能力由 2.0.0 升至 2.1.0，同步 package.json、package-lock.json、CHANGELOG 与 README；提交和推送目标为 origin/main。部署按 DEPLOYMENT.md 先推送，再从提交快照 rsync，远端先构建 frontend-builder、再构建并更新 backend；保留生产凭据和已有数据卷，不包含未跟踪的 pnpm-lock.yaml。本任务无后端或数据库变更，不触发迁移备份步骤。

开发阶段的测试和本地隔离页面结果见上文；发布阶段不重复本机构建/测试，不进行线上冒烟或功能验收。实际推送与远端部署进度见 [04-release.md](04-release.md)：代码已推送、远端前端构建成功，应用更新因 SSH 握手重置受阻；用户业务验收仍待执行。

bootstrap 只返回 legacy-needs-confirmation，未改原 AGENTS.md；原项目分层声明是示例模板，未当作已确认边界使用。

## 用户手动验收

- [ ] 登录真实环境，在归档页与首页的已归档词上检查默认模糊、点击揭示和发音；确认未归档词正常。
- [ ] 完成真实一组闪卡并多轮重练，确认只复习不认识/模糊词；评分、SRS 排期与记住后的归档按原规则生效。
- [ ] 用常用手机/浏览器检查布局和触摸点击，真实音频播放可用。

真实 API 落库、真实音频及手机布局未由本次隔离验证覆盖，交由用户验收；保持 awaiting_user。

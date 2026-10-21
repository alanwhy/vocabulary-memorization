<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-21 07:46 / 结束 2026-10-21 08:14 / 规范版本 @sw/dev-skills@1.1.2 -->
# 实现计划

## 背景
feature：派生词增量补全及多入口纠错。上游见 01-spec，接口与状态见 02b-design。

## 验证方式（验证前置）
- Go 测试 + race：旧数据/[]、模型请求、权限、重复预约、失败恢复、成功保存内容及复习状态不变。
- 前端构建、局部 ESLint 和 diff 空白检查通过。
- 本地页面测试数据：背单词/闪卡看见独立派生词；打开纠错空输入被拦，输入原因确定后全套新内容即时出现；故障保留内容与进度；旧闪卡补查后更新内容。
- codegraph affected backend/internal/model/models.go 实际输出：No test files affected；图未识别 Go 现存测试，手动纳入 backend/internal/model/models_test.go 和 backend/internal/app/deepseek_test.go。前端无现有测试框架，按操作路径验证。

## 改动清单
- [x] backend/internal/model/models.go：derivatives 字段、补全判断、合并保留；Go 领域测试。
- [x] backend/internal/app/deepseek.go、translate.go：新提示及纠错原因、结果校验，模型请求测试。
- [x] backend/internal/app/app.go、backend/internal/storage/words.go、dictionary.go：原子预约、派生词版本缓存补齐及强制更新及词键串行锁，Go 编译/race。
- [x] backend/internal/app/words.go、correction.go、routes.go：旧词/闪卡补全、强制纠错、失败保存旧内容；HTTP 测试。
- [x] backend/internal/model/models_test.go、backend/internal/app/word_enrichment_test.go、routes_test.go：缓存/旧数据/纠错成功失败/权限并发测试。
- [x] frontend/src/components/word/WordCorrectionButton.vue、WordCard.vue、WordList.vue、frontend/src/views/HomeView.vue、ArchiveView.vue：纠错事件、同步已有闪卡队列和派生词展示；构建/浏览器。
- [x] frontend/src/composables/useFlashcardQueue.js、frontend/src/views/FlashcardView.vue：内容轮询、派生展示、纠错与评分保护；浏览器进度验证。
- [x] 独立子智能体按 sw-review 完成 docs/ai/派生词展示与单词纠错/03-review.md。

## 关键文件
translateWord -> lookupDeepSeek；spawnTranslation -> translateAndSave -> MergeSensesByPos -> UpdateSenses/SaveSenses。WordList -> WordCard，FlashcardView -> useFlashcardQueue。

## 本次不做
实现阶段不发布、不提交推送；2026-10-21 浩原后续已授权提交、push 和上线，按 quick-release 发布 1.22.0。仍不批量改历史数据库，不自动把派生词加入学习队列，不改既有例句高亮规则。

## 风险
派生关系语义依赖模型质量；本地替身验证不能证明真实模型输出正确。首次旧词补齐仍需一次查询，后续缓存避免重复。

## 实际验证（2026-10-21）
- go test -race ./...：app/model/storage 全部通过。
- npm run build：2196 modules，成功；已有大包警告保留。
- 修改前端文件的 ESLint：通过；git diff --check 通过。
- ego-browser 本地生产构建 + 测试 API：背单词独立两类词展示、空原因拦截、输入原因全套更新、故障保留内容；闪卡空原因拦截、纠错成功仍为 1/1 和 ×7、翻面状态保留。
- HTTP 测试使用本地模型替身和 fake store，验证原始请求原因/完整内容、401/404/400/409/422/503/500、并发只发一次、缓存复用及独立 SRS 状态保留。未访问真实模型、生产数据库及线上页面。
- 本地闪卡纠错后选择“模糊”，本组完成显示模糊 1 个，结果列表保留新的完整释义。

- 复核补验：跨页背单词纠错后返回已加载闪卡，显示新例句/派生，位置 1/1 和 ×7/翻面保留；闪卡模拟故障同样保留内容与进度。
- 实际队列模块的受控异步验证：较早轮询在纠错后返回不覆盖新内容；pending 返回 derivatives 非空后自动合并并解除 translating，index/flipped 不变。
- 浏览器 pending 补全状态自动解除、翻面和 1/1 保留已核对；该轮测试 API 共享对象导致派生返回为 null，故不以该轮作为派生显示证据，派生显示由前两轮页面验证和实际模块响应验证覆盖。
- 最终 go test -race ./...、go vet ./...、局部 ESLint、npm run build 和 diff check 均通过。

- 最终旧闪卡全页面补验通过（本地测试 API 采用独立快照）：进入 pending 卡，派生词未显示；翻面后等待轮询，补全提示消失，派生词 worked/worker 自动出现且形近词 word 独立展示；flipped=true、进度 1/1、次数 ×7 保留。该条已补齐上一轮测试 API 缺陷导致的证据缺口。

## 发布交接
2026-10-21 用户明确授权提交、push 和线上部署。发布版本 1.22.0，更新 CHANGELOG 和 README，沿用 DEPLOYMENT 的 rsync + 远端 Compose 构建流程。无表结构改动、无启动时历史数据批量迁移；JSON 派生词在后续录入和复习时按需补齐。实现阶段的测试证据见上方；本次 quick-release 不重复本机构建/测试，也不执行线上冒烟和功能验收。

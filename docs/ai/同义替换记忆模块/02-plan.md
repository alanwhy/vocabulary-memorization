# 同义替换训练实现计划

阶段记录：GPT-6；2026-11-12；项目 frontend@1.23.0；规范 @sw/dev-skills@1.3.3。任务类型 feature，依据 [完整产品方案](README.md) 与 [详细设计](02b-design.md)。浩原已于 2026-11-12 明确授权实现；代码已完成。勾选表示实现完成，实际验证与待手动验收以 [03-review.md](03-review.md) 为准。

## 验证前置

实际开发前使用两个独立测试账号、独立 MySQL 测试库和可用的 DeepSeek 配置。只在隔离的测试环境生成内容和执行破坏性数据库用例。没有真实依赖时完成可运行的模拟检查，并记录真实验证缺口。

| 命令/操作路径 | 预期结果 |
|---|---|
| 独立测试库首次初始化，再执行第二次启动迁移 | 8 张新表出现且结构一致；重复迁移成功；已有单词、用户、闪卡排期不变 |
| 账号 A 登录 → 同义替换 → 选 2 个本人词 → 准备内容 → 开始练习 → 提交 → 下一题 → 完成本组 | 出现经过检查的关系与题目；解析完整；成绩、关系排期落库；原词字段不变化 |
| 粘贴完整阅读材料 → 提取关系 → 查看原文证据 → 选择确认 → 生成变式 → 作答 | 材料元信息保留，来源标签准确，关系基于原文与正确选项；上下文不足单独说明 |
| 在题目前查看响应，再使用词义提示并提交 | 未作答响应不含答案或解析；提示使用服务器记录；客户端不能自报未提示以获得独立掌握 |
| 同一题重复提交相同请求，再提交不同选项；刷新页面并跨路由返回 | 相同请求返回原结果，一次计分；不同提交冲突；恢复原题/解析和本组计数 |
| 使用模拟响应制造模型超时、429、格式错误、多解和单元部分成功 | 仅合格内容入库；失败原因明确；可用内容先练；重复点击和恢复不重复生成内容 |
| 任务处理中重启服务，再模拟旧租约迟到响应 | 未完成单元恢复；已保存单元跳过；旧租约不能写回；同题唯一键仍生效 |
| 同时请求多个准备任务、重试任务并跨 CST 日界测试配额 | 每人一个活动任务；新任务及实际模型请求额度按契约；失败与重试不能绕过限额 |
| 账号 B 访问 A 的材料、任务、关系、会话、题目反馈接口 | 均返回 404，不泄漏本人以外内容，不改变任何进度 |
| 反馈题目多解，再删除一份素材 | 争议题暂停、成绩作废、关系进度按有效历史回算；删除关联内容并处理活动任务/会话，不重新写回 |
| 连续两天使用不同题独立答对，再答错；测试不确定/提示分支 | 掌握条件、取消掌握和排期符合详细设计，不能一次答对即归档原词 |
| 运行 `go test ./...`（backend）与 `npm run build`（frontend） | 后端规则及兼容检查、前端构建完成；单独报告真实服务与手动验收结果 |

### 受影响测试定位

CodeGraph `affected` 对 app.go、migrations.go、AppTopbar.vue 均返回“ℹ No test files affected by the changed files.”，这是工具定位结果，不代表这些改动无兼容风险。

`impact Handler` 实际列出 routes_test.go、word_enrichment_test.go 与 middleware_test.go；现有后端有 Go testing 测试，前端 package.json 没有 test 命令。本任务新增必要的服务端规则和事务检查，不新增 UI 自动化套件；页面由浩原手动验收。

## 改动清单与顺序

每个条目先完成所列改动，再执行对应检查。下面阶段顺序表示依赖，不要求派生子智能体。

### A. 领域与存储

- [x] `backend/internal/model/training.go` —— 新增材料、关系、题目、任务、会话、进度和调用记录模型及枚举 —— 编译并核对 ID/可空字段与详细设计。
- [x] `backend/internal/training/validation.go` —— 实现输入、出题结果、引文匹配与盲审接受规则 —— 使用 validation_test.go 验证歧义、缺字段及超长边界。
- [x] `backend/internal/training/validation_test.go` —— 覆盖会让题目错误入库的结构与语义校验分支 —— 运行对应 Go 单测。
- [x] `backend/internal/training/scheduling.go` —— 映射实际作答到 SRS 评分，实现掌握条件与成绩回算 —— 使用时间固定的 scheduling_test.go。
- [x] `backend/internal/training/scheduling_test.go` —— 验证 24 小时、不同题、不确定、提示与取消掌握 —— 运行对应 Go 单测。
- [x] `backend/schema.sql` —— 按契约加入 8 张新表的新安装定义 —— 独立数据库初始化与键/默认值检查。
- [x] `backend/internal/storage/training_migrations.go` —— 新增幂等建表入口 —— 对独立旧版本测试库执行两次迁移。
- [x] `backend/internal/storage/migrations.go` —— Migrate 末尾调用新模块迁移 —— 确认旧迁移顺序和历史数据未改变。
- [x] `backend/internal/app/training_store.go` —— 声明应用层消费的窄存储接口 —— app 不引用具体 storage 实现，核对用户条件不可省略。
- [x] `backend/internal/storage/training.go` —— 实现材料、关系与题目的本人查询/写入/删除、题目反馈和进度回算事务 —— 真实测试库检查删除与跨用户过滤。
- [x] `backend/internal/storage/training_jobs.go` —— 实现任务幂等、租约、checkpoint、调用配额预留和用量记录 —— 并发事务测试重复任务、配额和旧租约写回。
- [x] `backend/internal/storage/training_sessions.go` —— 实现组建会话、私有快照、提示记录、作答事务与统计 —— 重复提交和重启读取保持一次计分。
- [x] `backend/internal/storage/training_test.go` —— 增加隔离 MySQL 集成用例及测试库开关 —— 缺测试库时明确跳过真实落库验证；配置后执行全部事务用例。

### B. 生成与任务

- [x] `backend/internal/training/prompts.go` —— 固定版本的提取、出题和盲审提示词 —— 对照真实使用边界，确保盲审输入未带预设答案。
- [x] `backend/internal/training/provider.go` —— 独立 DeepSeek client、超时、响应大小和 usage 解析 —— provider_test.go 的模拟服务覆盖异常。
- [x] `backend/internal/training/provider_test.go` —— httptest 覆盖正常、超时、429/5xx、401、格式错误与缺用量 —— 不依赖真实 API Key 执行单测。
- [x] `backend/internal/app/training_worker.go` —— 读取已有配置、持久化任务消费、逐单元验证保存、部分成功、重试与退出 —— worker 模拟测试并做一次真实小批量生成。
- [x] `backend/internal/app/training_worker_test.go` —— 租约、checkpoint、部分成功及服务取消 —— 检查无重复落库、已成功内容保留。
- [x] `backend/internal/app/app.go` —— 注入 TrainingStore，新增训练依赖和线程等待状态；旧测试允许未注入 —— 跑现有 app 测试，旧接口行为保持。
- [x] `backend/internal/app/lifecycle.go` —— 初始化时启动训练 worker，停止/等待时涵盖它 —— 有运行任务的关闭与重新启动检查。
- [x] `backend/cmd/vocab-server/main.go` —— 注入训练 repository —— 隔离环境启动与退出，确认 worker 使用同一连接池。

### C. HTTP 闭环

- [x] `backend/internal/app/training_handlers.go` —— 概览、关系、材料、任务、会话、提示、答题和反馈接口，严格投影未作答响应 —— training_test.go 验证完整 HTTP 路径。
- [x] `backend/internal/app/training_test.go` —— 登录隔离、答案隐藏、提示真实性、重复提交、会话恢复与反馈 —— Go HTTP 测试，必要事务验证引用 storage 用例。
- [x] `backend/internal/app/routes.go` —— 注册所有 /api/training 路由，沿用鉴权和请求超时 —— 现有 routes_test.go 与新接口检查。
- [ ] 已跳过（原因：新增路由鉴权与接口检查集中在 `backend/internal/app/training_test.go`，原路由用例保留，避免重复检查；后续：不另加重复用例）。

### D. 训练页面

- [x] `frontend/src/api/client.js` —— 仅命名导出已有 request 为 apiRequest，供新模块发送任务幂等请求头；原调用签名与行为不变 —— 构建并手动核对新旧入口登录过期处理一致。
- [x] `frontend/src/api/training.js` —— 新接口包装和 JSDoc 数据定义，创建任务携带稳定 Idempotency-Key —— 构建，并检查前端没有提交 correct/rating/hint_used。
- [x] `frontend/src/composables/useSynonymTraining.js` —— 概览、任务轮询、恢复、答题与提交互斥 —— 手动刷新/重复点/隐藏页面验证。
- [x] `frontend/src/views/SynonymTrainingView.vue` —— 独立学习入口及四个页内视图 —— 登录后能完整导航，主题和窄屏可读。
- [x] `frontend/src/components/training/TrainingDashboard.vue` —— 范围、难度、学习顺序和开始/恢复入口 —— 空状态与已有活动会话手动检查。
- [x] `frontend/src/components/training/TrainingPrepare.vue` —— 本人词选择及自选词种子 —— 1–5 项限制、错误词和已归档词行为。
- [x] `frontend/src/components/training/TrainingJobStatus.vue` —— 等待、部分可用、失败与重试 —— 对应实际任务状态，页面隐藏暂停轮询。
- [x] `frontend/src/components/training/TrainingRelationCard.vue` —— 新表达学习与提取证据确认 —— 多义词说明清晰，不把词群画成无条件同义。
- [x] `frontend/src/components/training/TrainingQuestion.vue` —— 选项、确定性、不知道、提示、提交与下一题 —— 作答前不出现答案，失败不推进。
- [x] `frontend/src/components/training/TrainingExplanation.vue` —— 正确映射与错误原因 —— 中文、英文引文准确对应且文本安全渲染。
- [x] `frontend/src/components/training/TrainingSummary.vue` —— 本组结果与错题回顾 —— 实际成功作答数与服务器一致。
- [x] `frontend/src/components/training/TrainingRelationList.vue` —— 关系本和易错关系分页 —— 初次无成绩、掌握状态与到期复习手动检查。
- [x] `frontend/src/components/training/TrainingMaterialForm.vue` —— 真题文本、选项、答案与来源表单 —— 缺答案、只有链接、超长字段提示明确。
- [x] `frontend/src/components/training/TrainingMaterialList.vue` —— 素材查看、提取、确认后生成与删除 —— 导入到练习全链路；删除时恢复/会话表现。
- [x] `frontend/src/router/index.js` —— 注册 /synonyms 登录路由 —— 未登录跳转，直接打开路径正常。
- [x] `frontend/src/components/layout/AppTopbar.vue` —— 新增同义替换入口，窄屏可换行 —— 保持已有背单词、闪卡、归档、统计入口。

### E. 收尾

- [x] `README.md` —— 写模块使用方法、内容来源标签、配置和排期区别 —— 与实际实现一致，不把生成题标真题。
- [x] `docs/ai/同义替换记忆模块/02-plan.md` —— 逐项写实际状态与验证缺口 —— 仅完成任务打勾，未验证与暂缓明确保留。
- [x] `docs/ai/同义替换记忆模块/03-review.md` —— 主智能体记录必要检查与手动验收清单 —— 遵照浩原减少独立复核的要求，不派生复核子智能体，不将等待用户验收写为已通过。

## 关键文件与调用关系

| 文件 | 改动与现有依赖 |
|---|---|
| app/app.go、cmd/vocab-server/main.go | 组合根注入新存储接口；已有 Dependencies 构造保留兼容 |
| app/lifecycle.go | Initialize 已先 loadSettings；训练 worker 在此之后启动；CancelBackground/WaitBackground 管理关闭 |
| app/settings.go | 只调用 getDeepSeekConfig；CodeGraph 显示既有查词、纠错、管理配置都依赖它，本次不改方法语义 |
| app/deepseek.go、app/translate.go | 保留词典专用流程；新模块独立 provider 与提示词，不向其塞入练习输出 |
| model/models.go | 调用现有 ApplySRSScheduling；不改 Word、Sense 或旧闪卡归档规则 |
| storage/migrations.go、schema.sql | 幂等迁移与新安装结构保持一致，只新增训练表 |
| frontend/router/index.js、AppTopbar.vue | 新增登录入口；CodeGraph callers AppTopbar 指向 App.vue，无需重写 App.vue |
| frontend/api/client.js | 已核对 apiPost 不支持自定义头，新增既有 request 的命名导出供任务幂等使用；这是全应用通用能力暴露，不更改旧包装、鉴权或错误行为 |

`codegraph callees translateWord` 指向 lookupDeepSeek、translateResult、deepseekConfig；新模块沿用配置来源，主动避免耦合其词典输出。上述调用关系已实际查明，未运行模块代码。

## 已收敛与暂缓事项

| 事项 | 状态 | 结论 |
|---|---|---|
| 考研一/二 | 已结论 | 默认通用，模块内可选，不阻塞实现 |
| 学习顺序 | 已结论 | 默认先学少量新表达，提供直接练习 |
| 内容来源 | 已结论 | 个人词库/自选词生成与真题文本提取均做；标识分开 |
| 初始题库 | 已结论 | 不声称已有真题库；从本人词汇或输入种子启动，题目经检查落库 |
| 数据共享 | 已结论 | 第一版全部个人隔离，同一用户复用缓存 |
| 自动搜索供应商 | 暂缓 | 不影响文本导入闭环；第一版手动验收后再选搜索与获取服务 |
| PDF/OCR、整套试卷、写作评分 | 暂缓 | 第一版短句与关系训练验收后再评估 |
| 真实模型与 MySQL 验证 | 实施时验证 | 本轮不读取或使用凭证；实现时沿用项目配置，缺少依赖则明确记录实测缺口 |

## 当前交付状态

模块代码已实现。Go 测试、应用/规则竞态检查、前端构建、ESLint、真实 DeepSeek 小批量生成与盲审通过；真实 MySQL 测试因缺少隔离环境明确跳过，页面与完整真实业务由浩原手动验收。没有提交、发布或归档。详细记录见 03-review.md。

## 实际增加的文件与配置

- [x] `backend/internal/storage/training_schema.sql` —— 将幂等新表定义作为 Go 嵌入资源；新安装 schema.sql 同步同一内容。真实迁移验证待隔离 MySQL 环境。
- [x] `backend/internal/training/live_test.go` —— 显式启用的真实 provider 检查，默认跳过；实际已通过一次小批量生成与盲审。
- [x] `.env.example`、`docker-compose.yml` —— 传递训练预算与后台配置；仅修改配置，未执行部署。
- [x] `frontend/src/assets/training.css` —— 新模块限定范围的主题与窄屏样式；页面体验待手动验收。

## 简化改造（最新执行项）

依据 04-simplification.md，直接继续实现。

- [x] 删除复杂题库页面和训练持久化调用，保留历史用户数据。
- [x] 实现本人未归档词按次数排序、同日下一批与跨日清空。
- [x] 实现内存中的当前练习、临时作答、并发去重与归档过滤。
- [x] 接入 DeepSeek 原生 Search/Fetch 和真题引用校验。
- [x] 简化页面并自动请求第一组，跨日可见时自动刷新。
- [x] 必要规则、provider 和真实本地服务检查；页面交用户验收。

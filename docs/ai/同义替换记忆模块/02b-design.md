# 同义替换训练详细设计

阶段记录：GPT-6；2026-11-12；项目 frontend@1.23.0；规范 @sw/dev-skills@1.3.3。执行范围见 [01-spec.md](01-spec.md)，产品行为见 [README.md](README.md)。本文契约已实现，实际细化见 03-review.md；示例 ID、时间和结果均为说明用数据，数据库实测和页面验收尚未完成。

## 1. 接口约定

所有接口由本项目 Go 后端提供，统一前缀 `/api/training`，均要求现有登录鉴权。用户 ID 由登录态取得，不接受请求指定其他用户。跨用户资源返回 404。

- 新模块 ID 在 JSON 中使用十进制字符串，对应数据库 BIGINT UNSIGNED；已有 word_ids 沿用 INT 数字。
- 日期用 RFC3339，业务日期与排期按 Asia/Shanghai；数据库沿用现有本地 DATETIME 约定，不修改旧时间逻辑。
- 页码 page 默认 1，page_size 默认 20、最大 50；响应 `{items:[],total:0,page:1,page_size:20}`，集合不返回 null。
- exam_track：general / english_one / english_two，默认 general；general 练习包含全部本人内容，指定一/二包含对应类别及通用内容。
- difficulty：basic / advanced，默认 basic；mode：daily / review。
- 创建任务使用 `Idempotency-Key`（1–64 字符）。同用户同键同内容返回原任务，同键不同内容 409。重复返回不消耗新任务额度。
- 普通 HTTP 处理沿用 10 秒请求超时；模型工作异步执行。请求正文最多 128 KiB。
- 统一错误 `{error:"面向用户的中文说明",code:"稳定错误标识"}`；新接口单独定义，不改变旧 writeError 返回。
- 400 格式或枚举错误；401 未登录；404 无可访问资源；409 重复冲突、已有活动任务或状态冲突；422 缺少必要上下文/选项不合法；429 请求额度不足；503 新生成未配置或模块依赖不可用。
- 模型禁用只阻止新生成；已保存题目仍可练习。拒绝生成前不创建空任务。

### 接口及样例

| 方法与路径 | 请求例子 | 成功响应例子与关键行为 |
|---|---|---|
| GET /overview | 无正文 | 200 `{due_relations:6,new_relations:4,active_session_id:null,active_job:null,stats:{graded:20,correct:14,independent_correct:10,hinted:3},generation:{enabled:true,jobs_remaining:3,calls_remaining:60}}`；没有训练历史时全部计数为 0 |
| GET /relations | `?scope=all&status=active&material_id=2&page=1&page_size=20` | 200 `{items:[{id:"7",anchor:"account for",meaning:"解释原因",replacement:"explain",status:"active",mastered:false,due_at:null,source_kind:"vocabulary"}],total:1,page:1,page_size:20}`；scope 为 all/due/difficult；material_id 可不传，status 默认 active，可查 draft/confirmed |
| GET /relations/{id} | 无正文 | 200 `{id:"7",meaning:"解释原因",content:{original:"The report accounts for the decline in sales.",paraphrase:"The report explains why sales fell.",restriction:"此处解释原因；表示占比时使用其他义项"},source:{kind:"vocabulary",material_id:null},progress:null}`；此处是学习卡，属于明确查看解析，不伪装成测试 |
| POST /jobs | `{kind:"generate_vocabulary",word_ids:[12,18],exam_track:"general",difficulty:"basic"}` 或 `{kind:"generate_vocabulary",seeds:["account for"],exam_track:"general",difficulty:"basic"}` | 202 `{id:"11",status:"queued",phase:"preparing",relation_count:0,question_count:0}`；word_ids 与 seeds 二选一，各最多 5 条；本人词、正常释义可用，错误/查询中词拒绝并列出原因 |
| POST /jobs | `{kind:"extract_material",material_id:"2"}` 或 `{kind:"generate_relations",relation_ids:["7","8"],difficulty:"basic"}` | 202 同上；extract_material 仅产生待确认关系，generate_relations 仅接受本人 confirmed/active 关系，最多 5 条 |
| GET /jobs/{id} | 无正文 | 200 `{id:"11",status:"running",phase:"verifying",total_units:5,finished_units:2,relation_count:2,question_count:4,draft_relation_ids:[],failures:[],retry_allowed:false,usage:{requests:4,prompt_tokens:null,completion_tokens:null}}`；用量未知为 null，不当作 0；有 question_count 时可开始练习 |
| POST /jobs/{id}/retry | `{}` | 202 `{id:"11",status:"queued",retry_units:2}`；仅重跑失败单元，已保存题目保留；终态 completed/cancelled 不可重试；重新检查配置和当日调用额度 |
| POST /materials | `{title:"阅读材料示例",exam_track:"english_one",year:2024,question_no:21,reference_url:"https://example.org/source",payload:{passage:"The report accounts for the decline in sales.",stem:"What does the report do?",options:[{id:"o1",text:"Explains why sales fell."},{id:"o2",text:"Causes sales to fall."},{id:"o3",text:"Predicts that sales will rise."},{id:"o4",text:"Shows that sales stayed unchanged."}],correct_option_id:"o1",explanation:"材料提供的解析，可省略"}}` | 201 `{id:"2",source_kind:"exam_material",source_status:"user_provided"}`；这是原创样例，不是 2024 真题；source_status 不被模型升级为“官方认证” |
| GET /materials | `?page=1&page_size=20` | 200 `{items:[{id:"2",title:"阅读材料示例",exam_track:"english_one",year:2024,relation_count:0}],total:1,page:1,page_size:20}`；列表不返回全文 |
| GET /materials/{id} | 无正文 | 200 `{id:"2",title:"阅读材料示例",reference_url:"https://example.org/source",source_status:"user_provided",payload:{passage:"The report accounts for the decline in sales.",stem:"What does the report do?",options:[{id:"o1",text:"Explains why sales fell."},{id:"o2",text:"Causes sales to fall."},{id:"o3",text:"Predicts that sales will rise."},{id:"o4",text:"Shows that sales stayed unchanged."}],correct_option_id:"o1"},draft_relation_ids:["7"]}`；返回保存的完整材料 |
| DELETE /materials/{id} | 无正文 | 200 `{deleted:true,removed_relations:2,removed_questions:4}`；删除关联关系和题目，取消相关任务，历史会话对应题标为内容已移除并从统计排除 |
| POST /relations/confirm | `{relation_ids:["7","8"]}` | 200 `{confirmed_ids:["7","8"]}`；最多 5 条；仅确认本人 draft；已确认同 ID 幂等；确认后由 POST /jobs 准备变式题 |
| POST /sessions | `{mode:"daily",exam_track:"general",difficulty:"basic",learn_first:true}` | 201 `{id:"23",status:"active",total:8,answered:0,learning_cards:[{relation_id:"7",anchor:"account for",meaning:"解释原因",replacement:"explain"}],current_question:{item_id:"31",ordinal:1,kind:"paraphrase_choice",stem:"The report accounts for the decline in sales.",options:[{id:"o3",text:"..."},{id:"o1",text:"..."},{id:"o4",text:"..."},{id:"o2",text:"..."}],source_label:"生成练习",hint_used:false}}`；有活动会话返回 200 原会话；学习卡最多 3 条；没有可用题返回 200 `{id:null,total:0,reason:"no_ready_questions"}`，不产生空会话 |
| GET /sessions/{id} | 无正文 | 200 与上相同，附 `last_answer`（已提交时）供恢复解析；只返回当前题与已答题信息，不返回未来答案。completed 时 current_question 为 null，返回已答题结果；被暂停的题自动标 unavailable 并移动到下一可用项 |
| POST /sessions/{id}/items/{item_id}/hint | `{}` | 200 `{hint:"account for 在此处指解释原因",hint_used:true}`；服务器先记录提示使用，再返回提示；重复请求幂等，答案映射不作为提示返回 |
| POST /sessions/{id}/items/{item_id}/answer | `{selected_option_id:"o1",confidence:"certain"}`；不知道为 `{selected_option_id:null,confidence:"uncertain"}` | 200 `{correct:true,rating:"good",hint_used:false,next_due_at:"2026-11-13T09:00:00+08:00",feedback:{correct_option_id:"o1",mappings:[{original:"accounts for",replacement:"explains",note:"解释原因"}],option_explanations:[{id:"o1",text:"保留解释关系"},{id:"o2",text:"解释改成造成"},{id:"o3",text:"时间与趋势改变"},{id:"o4",text:"下降改成不变"}]},session:{id:"23",answered:1,total:8}}`；同项同答案重复提交返回原结果，不重复排期；同项不同答案 409；服务器不接收 correct/rating/hint_used 自报字段 |
| POST /questions/{id}/feedback | `{reason:"multiple_answers",note:"两个选项似乎都能成立"}` | 200 `{paused:true,voided_attempts:1}`；reason 为 multiple_answers/explanation_wrong/unnatural/source_issue/other，note 1–500 字；暂停本人题、作废其成绩并重算关系进度 |

GET /relations?scope=difficult 返回存在错误、不确定或用提示的活跃关系，可附最近一次有效答题摘要；详情及 completed 会话给已答题解析，满足错题回顾，无需另建错题内容表。

### 导入边界

title 1–120 字；passage 1–12000 字；stem 1–1000 字；四个 option.text 各 1–1000 字且规范化后不重复；explanation 最大 4000 字，可缺省。correct_option_id 必须引用选项。year 允许 null 或 1980 至客户端当前年份，question_no 允许 null 或 1–100；字段不作为真实性证据。

reference_url 允许 null 或最多 2048 字的 http/https URL，仅存储、不请求远程地址。第一版不支持只有链接、扫描图片或缺少正确答案的材料自动提取。

## 2. 数据库存储

新增 8 张表，前缀 training_，在 `backend/schema.sql` 添加新安装定义，`backend/internal/storage/training_migrations.go` 定义启动迁移，由现有 Migrate 末尾调用。均使用 InnoDB/utf8mb4；历史表与记录不回写。迁移使用 CREATE TABLE IF NOT EXISTS，新功能上线时空表即合法初始状态。

统一约定：id BIGINT UNSIGNED AUTO_INCREMENT 主键；user_id INT NOT NULL 外键 users(id) ON DELETE CASCADE；created_at/updated_at DATETIME NOT NULL 由服务端写入；下表单列“可空”之外字段均非空。VARCHAR 状态列由应用白名单验证。

| 表 | 关键字段、默认值和限制 | 索引与关系 |
|---|---|---|
| training_materials | title VARCHAR(120)；exam_track VARCHAR(16) 默认 general；year SMALLINT UNSIGNED 可空；question_no SMALLINT UNSIGNED 可空；reference_url VARCHAR(2048) 可空；payload JSON；source_status VARCHAR(24) 默认 user_provided | 主键 id；(user_id,created_at,id) |
| training_relations | material_id BIGINT UNSIGNED 可空；anchor_word_key VARCHAR(255)；meaning VARCHAR(200)；exam_track VARCHAR(16) 默认 general；difficulty VARCHAR(16) 默认 basic；content JSON；fingerprint CHAR(64)；status VARCHAR(16) 默认 draft；origin VARCHAR(24) 为 vocabulary/exam_material；model_name VARCHAR(128)；prompt_version VARCHAR(32) | UNIQUE(user_id,fingerprint)；(user_id,status,exam_track,id)；material_id 外键 materials ON DELETE CASCADE |
| training_questions | relation_id BIGINT UNSIGNED；kind VARCHAR(24)；version INT UNSIGNED 默认 1；content JSON，包含题干、选项、正确答案、映射和解释；content_hash CHAR(64)；status VARCHAR(16) 默认 active；model_name VARCHAR(128)；prompt_version VARCHAR(32) | UNIQUE(user_id,content_hash)；(user_id,relation_id,status,id)；relation_id 外键 relations ON DELETE CASCADE；内容修订新增题 ID，旧题暂停 |
| training_jobs | idempotency_key VARCHAR(64)；request_hash CHAR(64)；kind VARCHAR(32)；payload JSON；checkpoint JSON 默认由应用写 `{}`；status VARCHAR(16) 默认 queued；phase VARCHAR(16) 默认 preparing；relation_count/question_count INT UNSIGNED 默认 0；lease_token VARCHAR(64) 可空；lease_until DATETIME 可空；error_code VARCHAR(64) 可空；error_message VARCHAR(500) 可空；finished_at DATETIME 可空 | UNIQUE(user_id,idempotency_key)；(status,lease_until,id)；(user_id,created_at,id)；普通可空 active_user_id 由事务在 queued/running 时写 user_id，终态清空，UNIQUE 保证一用户一个活动任务 |
| training_model_calls | job_id BIGINT UNSIGNED；unit_key VARCHAR(128)；stage VARCHAR(16)；attempt TINYINT UNSIGNED；status VARCHAR(16) 默认 reserved；prompt_tokens/completion_tokens INT UNSIGNED 可空；started_at DATETIME；finished_at DATETIME 可空；error_code VARCHAR(64) 可空 | (user_id,started_at,id) 用于每日配额；job_id 外键 jobs ON DELETE CASCADE；不保存 API Key、完整原始响应或原文日志 |
| training_sessions | mode VARCHAR(16)；exam_track VARCHAR(16)；difficulty VARCHAR(16)；status VARCHAR(16) 默认 active；total SMALLINT UNSIGNED；finished_at DATETIME 可空 | (user_id,created_at,id)；普通可空 active_user_id 由事务在 active 时写 user_id，完成时清空，UNIQUE 保证一用户一组活动练习 |
| training_session_items | session_id BIGINT UNSIGNED；question_id/relation_id BIGINT UNSIGNED 可空；ordinal SMALLINT UNSIGNED；snapshot JSON（固定题目版本与选项顺序，包含服务端私有答案）；grade_status VARCHAR(16) 默认 pending；selected_option_id VARCHAR(8) 可空；confidence VARCHAR(16) 可空；hint_used TINYINT(1) 默认 0；correct TINYINT(1) 可空；rating VARCHAR(16) 可空；request_hash CHAR(64) 可空；submitted_at DATETIME 可空 | UNIQUE(session_id,ordinal)；(user_id,relation_id,submitted_at,id)；session 外键 CASCADE；question/relation 外键 SET NULL；客户端响应通过投影移除答案，不能直接输出 snapshot |
| training_progress | 无自增 id；user_id INT、relation_id BIGINT UNSIGNED 复合主键；due_at DATETIME 可空；interval_days INT UNSIGNED 默认 0；ease_factor DECIMAL(4,2) 默认 2.50；eligible_streak INT UNSIGNED 默认 0；last_success_question_id BIGINT UNSIGNED 可空；last_success_at/mastered_at DATETIME 可空；updated_at DATETIME | (user_id,due_at,relation_id)；relation 外键 CASCADE；last_success_question_id 外键 SET NULL |

关系 content 保存 `{anchor,pos,meaning,replacement,original,paraphrase,restriction,evidence}`；evidence 为材料中可精确找到的引文和对应选项引用。题目 mappings 每个 original 引文必须出现在题干中、replacement 出现在正确选项中，必要时带 occurrence 指定第几次出现，由服务端定位高亮。

fingerprint 是锚点、义项、使用限制、材料 ID 与提示词版本的规范化 SHA-256；它用于精确缓存去重，不声称能识别所有语义等价关系。content_hash 包含完整题干、选项内容及答案；相同内容不会重复落库，内容变更不覆盖既有作答版本。

每个写入都验证父资源属于同用户；Repository 查询和更新必须显式带 user_id 条件，不能只依赖单个数字 ID 或外键。删除用户沿用 users 的级联行为。

材料删除采用事务：取消引用材料的活动任务并撤销租约，清空其请求 payload；清空关联会话项 snapshot 并标 unavailable，删除材料及级联内容/进度；失效项不计统计。后台写回前同时验证租约和材料存在，防止删除后又写回生成结果。

## 3. DeepSeek 与后台任务

### 接入边界

- 复用 `getDeepSeekConfig()` 的 BaseURL、APIKey、Enabled 和 FallbackModel；新模型服务放 `backend/internal/training/provider.go`。
- 请求使用现有兼容的 chat/completions 通道，单独 HTTP client，默认超时 60 秒，响应体最多 1 MiB；不把任务延长到普通 HTTP 请求中。
- 新模块不改 lookupDeepSeek 的词典提示词，不调用 translateWord 去出题，也不直接启用目前仅保存配置的 ThinkingModel。
- `TrainingStore` 窄接口在 `backend/internal/app/training_store.go` 声明，具体 MySQL repository 由组合根注入到 Dependencies.Training。
- 原有测试没注入 Training 时旧接口照常工作，新模块返回不可用；生产组合根必须注入。Initialize 在加载配置后启动工作线程，训练线程单独 WaitGroup，CancelBackground 和 WaitBackground 同时涵盖两类工作。

### 提示词与验证

生成系统提示词使用版本 `training-v1`，输入材料和用户反馈作为 JSON 数据传入，不能覆盖任务规则。每个生成单元限一个义项、一条关系、最多两道题。

强制要求：四个选项；唯一正确答案；基本词性和长度相当；正确选项保留原文主体、时间、范围、程度、可能性、否定和因果角色；错误选项分别指出改动；中文解析 1–500 字；1–6 组精确引文映射。基础题以单句、约 35 个英文词以内为目标；进阶原文 1–3 句、总计约 120 词以内，长度超限由结构验证拒绝，不通过截断破坏语义。

候选提取提示词仅输出可由原文和正确选项直接支持的关系；上下文不足、只有主旨推断或答案冲突时返回空候选和具体原因。基于 confirmed 关系生成时，将用户确认的义项、两个表达和材料证据作为不可改写的关系约束；模型不能换成另一个义项来凑题，违反时拒绝该单元。

盲审使用新对话、temperature 0，输入原文、选项和使用义项，隐藏生成方的正确答案与解释，要求返回 `{valid_option_ids:[...],issues:[...]}`。仅一个 ID 有效且与预设答案一致，并且语义检查无阻断问题时接受；多解、无解或不一致均不入库。同模型的新上下文称“盲审”，不宣传为独立来源事实认证。

`ValidateQuestion` 先检查 JSON、字段长度、选项唯一、答案引用、各项解析和引文匹配；`AcceptReview` 再检查盲审结果。原始错误内容不放进用户解析，也不打印凭证或材料全文。

### 状态与恢复

job.status：queued → running → completed/partial/failed；材料删除为 cancelled。phase：preparing/generating/verifying/finished。partial 是终态，表示有已保存内容但至少一个单元失败；running 时也可以已有可用内容。

1. 创建任务先验证输入、配置和配额，数据库记录后立即返回任务 ID。
2. worker 通过 MySQL 8 的事务行锁与租约取得一个任务；默认全局并发 1，每 5 秒检查待处理任务。独立于旧查词信号量。
3. 词库路线读取本人词义快照；材料路线先提取 draft 关系，展示证据等待用户选择确认，提取任务在此结束。
4. 关系生成单元调用生成与盲审，成功后在同一事务保存关系/题目、更新计数和单元 checkpoint。
5. 网络超时、429、5xx 最多重试一次，退避 2 秒或采用不超过 30 秒的 Retry-After；401/403、结构不合格和语义多解不自动循环重试。手工重试仍受额度控制。
6. lease 默认 180 秒；每次调用前/后续租；写回必须比较 lease_token，旧任务持有者不能覆盖接管结果。
7. 重启后接管租约过期的 running 单元，跳过已提交 checkpoint；SQL 唯一键避免重复内容，即使重复远端调用也不重复题目。
8. 退出时取消请求与等待，保持已提交内容；未完成单元下次恢复。不能承诺网络中断情况下模型计费恰好一次。

### 配额与用量

默认部署配置：`TRAINING_DAILY_JOBS=3`、`TRAINING_DAILY_CALLS=60`、`TRAINING_MAX_ANCHORS=5`、`TRAINING_WORKERS=1`、`TRAINING_MODEL_TIMEOUT_SECONDS=60`。非法配置回退默认值，不修改现有查词配置。

ConsumeCallBudget 在用户行锁下，按 CST 当天统计 training_model_calls 并预留记录，防止并发绕过；预留亦计入上限，确保实际发出的请求不超过上限。失败、提取、生成、盲审和重试都消耗请求额度。响应 usage 未提供时 token 列为 NULL。3 个新任务额度包含提取与生成；重试现有任务不计新任务，但仍计模型请求。

## 4. 练习与排期事务

创建会话先恢复活动会话；无活动时按用户行锁选题并保存快照。每日组优先 6 个到期关系、4 个新关系，review 仅取到期关系；每组最多 10 个不同关系。有剩余题时可以补到 10，无题不建会话。

优先挑选该关系本人未见或最久未见的题，避免连续同一道；四项选项随机排序并在 snapshot 固定，前端按当前位置显示 A/B/C/D，提交稳定 option_id。

提交答案流程：

1. 锁定本人会话项，验证当前题顺序、状态及选项 ID。
2. 已提交且 request_hash 一致则返回原结果；不一致返回 409。题目已暂停/删除则返回状态冲突并提供刷新入口。
3. 从 snapshot 私有答案评分，从服务器提示记录读取 hint_used。
4. 规则：错误/不知道/用提示 → again；正确但不确定 → fuzzy；正确且确定、未用提示 → good。
5. 锁定或建立 training_progress，调用现有 model.ApplySRSScheduling；写入排期和掌握状态，不更新 words。
6. good 且距上次独立成功至少 24 小时、question_id 不同才累计 eligible_streak；首次 eligible_streak 为 1。达到 2 标 mastered。其他评分清零 eligible_streak/mastered_at，again 把间隔重置为 1 天。
7. 保存作答结果与会话完成状态，事务提交后返回解析。提交失败整个事务回滚，不推进题序。

模型错误反馈事务暂停题、将关联 graded 记录标 voided，按其余有效作答时间顺序重新计算该关系进度；没有剩余成绩则清除进度。pending 题标 unavailable，活动会话可继续其余题。已有历史正确与更正后的题目版本分别保存，不能静默改答案。

统计从有效 graded 会话项聚合：普通正确率=correct/graded；独立答对比例=正确且确定且无提示/graded；提示使用比例=hinted/graded。分母为 0 显示暂无记录。按近 7 日、累计两个范围查询，不下载所有历史到浏览器。

## 5. 前端设计

新增 `/synonyms` 登录后路由，导航显示“同义替换”。第一版页内使用学习、关系本、错题回顾、真题素材四个视图，不新建复杂全局菜单。

| 页面/组件 | 输入/事件 | 职责 |
|---|---|---|
| views/SynonymTrainingView.vue | 入口页 | 组合首页、活动会话、任务和内容列表；使用现有主题 |
| components/training/TrainingDashboard.vue | overview；start/prepare/import/resume | 展示待复习及最近结果，选择考试范围、难度和学习顺序 |
| components/training/TrainingPrepare.vue | 本人词列表；submit | 最多 5 个锚点，或最多 5 个自选词；清晰显示等待与实际可用数量 |
| components/training/TrainingJobStatus.vue | job；retry/start | 进度按已完成单元显示，不编造总体预计时间；区分失败原因 |
| components/training/TrainingRelationCard.vue | relation；dismiss/confirm | 义项、表达、语境、限制和材料证据；兼用于导入结果确认 |
| components/training/TrainingQuestion.vue | question、submitted；answer/hint/feedback/next | 四选一、确定性、不知道、提示；提交前隐藏解释，成功后允许下一题 |
| components/training/TrainingExplanation.vue | feedback | 对应表达与逐项原因，内容作为文本渲染，不执行模型 HTML |
| components/training/TrainingSummary.vue | completed session | 回顾错误/不确定/提示使用，查看已提交解析 |
| components/training/TrainingRelationList.vue | scope、items；open | 分页关系本与易错关系，不把普通正确率等同掌握度 |
| components/training/TrainingMaterialForm.vue | 可选 draft；submit | 文本、四项选项、正确答案、出处元信息；提交前检查长度与必填 |
| components/training/TrainingMaterialList.vue | items；open/extract/delete | 材料摘要和全文、提取任务、关系确认入口 |

组件按实现实际复杂度可合并简单纯展示部分；关键状态仅存 composables/useSynonymTraining.js，不分散保存答案状态。

状态以服务端为准：overview、activeJob、activeSession、selectedOption、confidence、requestError、loading。路由切换保留本轮选项；刷新后 GET overview 再恢复活动会话，已提交项恢复解析，不再提交。题库、答案和进度不存浏览器作为唯一持久化。

任务页面可见且任务未结束时每 3 秒查一次，未变化逐渐到 10 秒；页面隐藏暂停，返回立即查询。轮询失败保留上次任务和进度并提供重试，不清空已生成内容。

解析中的查词采用现有能力；答题阶段查看提示必须调用提示接口留下服务器记录。仅打开学习卡不伪装成提示未用的测试；结果文案区分“学习表现”和“已掌握”。

本项目为 JavaScript，不新增 TypeScript。请求/响应、枚举和 ID 类型用 `frontend/src/api/training.js` 的 JSDoc 定义；常规接口复用 apiGet/apiPost/apiDelete。现有 apiPost 不支持额外请求头，因此 client.js 仅新增 `apiRequest = request` 命名导出，training.js 用它携带创建任务的 Idempotency-Key；复用现有鉴权与 401 处理，不复制请求逻辑。既有请求包装签名、错误文案和返回保持不变。

概览的 due/new 计数只包含至少有一道 active 题的可练关系；draft、仅 confirmed 尚未出题及全部题暂停的关系单独在内容准备列表显示，避免首页显示待练数量却没有可出题内容。

## 6. 必要验证

| 被测对象与文件 | 重点覆盖 |
|---|---|
| internal/training/validation_test.go | 多义词、短语、四项去重、答案缺失、引文不匹配、多解/盲审不一致、非法枚举和超长输入 |
| internal/training/scheduling_test.go | 不知道/提示/不确定；24 小时与不同题条件；答错取消掌握；初学正确不等于长期掌握 |
| internal/training/provider_test.go | 用 httptest 模拟正常、格式错误、429/5xx/401、超时、缺 usage；确保无真实模型请求的稳定单测 |
| internal/storage/training_test.go | 使用真实独立 MySQL 测试库验证幂等迁移、锁与唯一键、重复计分、预算抢占、跨用户过滤、删除及争议成绩回算；测试库未提供时明确未验证 |
| internal/app/training_test.go | 模块完整 HTTP 路由、登录隔离、提示记录、答案不提前返回、重复提交、会话恢复与反馈 |
| internal/app/training_worker_test.go | 已提交 checkpoint 恢复、部分成功、租约过期、旧租约写回失败、取消与退出 |
| 现有 routes_test.go、相关已有后端测试 | 保持现有路由、查词、词条纠错和闪卡行为兼容 |

不新增机械镜像式 UI 测试，也不自动启动独立复核子智能体。必要后端规则检查完成后，页面、阅读负担与真实生成内容由浩原手动验收；实际 DeepSeek、MySQL 落库和页面检查的结果单独记录，不能用构建成功代替。

## 实际细化记录

2026-11-12：任务响应增加 material_id；关系详情增加最近有效作答；会话保存学习顺序、新关系标记与评分结果快照；种子快照用于精确缓存，题目 hash 包含关系 ID。每道题单独盲审。已确认关系沿用其范围与难度。细节见 03-review.md，均为本模块闭环所需细化，不新增搜索或共享题库范围。

2026-11-12 本地启动修正：MySQL 8 不允许在存储生成列所依赖的 user_id 上设置级联外键，活动用户唯一列改为普通可空列，由创建、重试、完成和取消事务维护；保留唯一索引与用户行锁，接口语义不变。

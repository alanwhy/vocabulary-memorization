执行模式：仅调研与需求设计（来源：浩原明确要求“调研是否可以做到”；本轮不进入计划、编码或发布）。

2026-11-12 阶段确认：浩原确认开始编码；包含 Search/Fetch、不含语音；采用普通用户 5 元/天、管理员默认不限额且可编辑、上线后即时开始记账。补齐计划后直接进入已授权的实现。用户管理列表仅用户名、角色、操作（详情/编辑/删除），详情展示原统计及额度，编辑提供用户名、角色、状态、额度和重置密码。按用户要求不启动独立复核。

# 用户每日模型费用额度可行性调研

阶段记录：模型 GPT-6（当前会话未提供精确子型号）；2026-11-12 11:08–11:12，Asia/Shanghai；项目 frontend@1.25.1；规范版本未提供。

## 需求陈述与结论

可实现每个账号每天按人民币限制大模型费用，管理员在用户管理中调整额度，管理员账号可以不限额。建议普通用户默认 5 元/天，管理员默认不限额；“5 元”在原需求中是示例，正式默认值仍可调整。

现有项目具备账号识别、管理员权限、用户管理页面和 MySQL 存储，但没有完整的费用账本或每日金额限制。需要新增后端计费与额度控制，不能只在前端禁用按钮。DeepSeek 文档中的 user_id 支持安全、缓存、调度隔离，并未提供业务用户按人民币设置日限额的参数；本项目需要自行实现。

可以严格控制的是按本项目确认的费率与计费规则计算的额度。供应商实际账单还受费率调整、时段判定和异常请求账单信息可得性影响，不能在未核对这些规则时承诺与供应商扣款完全一致。

## 已确认现状

| 位置 | 当前行为 | 对本需求的影响 |
|---|---|---|
| backend/internal/model/models.go:229 | User 有 ID、IsAdmin、Disabled，没有额度字段 | 可沿用用户身份，需新增额度配置 |
| frontend/src/components/admin/AdminUsers.vue | 用户列表支持创建、重置密码、启停、删除 | 额度编辑可直接落在该页面 |
| backend/internal/app/deepseek.go:20、26、37 | 直接调用 chat/completions；请求未设置 max_tokens；响应只解析 choices | 查词缺少 usage、输出上限和计费回调 |
| backend/internal/app/translate.go:19 | translateWord 返回释义及拼写状态，错误被转成 Source=none | 需保留“额度不足”错误，不能被当成普通失败重试 |
| backend/internal/app/words.go:116、186 | 异步查词、重试、恢复未完成任务；共享词库命中后不调用模型 | 后台任务必须携带付费用户身份；缓存命中不扣费 |
| backend/internal/app/correction.go:61、128 | 用户纠错强制重新调用模型，不复用缓存 | 必须计入触发用户的日额度 |
| backend/internal/app/dictionary.go:114 | 管理员重新查询母库词条 | 记到操作管理员，默认不限额但保留费用统计 |
| backend/internal/training/provider.go:33、67 | 出题客户端解析 prompt/completion tokens | 可复用部分 usage 结构，仍缺缓存细分及统一记账 |
| backend/internal/app/training_worker.go:20、39、54 | 每词生成及复核各调用一次模型，返回 usage 被丢弃 | 两次真实请求都要记账，不能把一道题按一次计费 |
| backend/internal/training/search.go:33、108 | 练习还调用 DeepSeek Search，必要时 Fetch；没有计费记录 | 如果目标是 DeepSeek 总支出，工具费用也应纳入 |
| backend/internal/training/daily.go:167、232 | 按用户准备十个词，并发生成；当日队列仅存内存 | 练习队列不能兼作金额账本，服务重启不能清空已用额度 |
| backend/schema.sql；backend/internal/storage/migrations.go | MySQL InnoDB；启动执行幂等迁移 | 可用数据库事务保障跨进程额度一致性 |

当前源码里的默认模型名为 deepseek-v4-flash，后台可改 BaseURL、模型及 Key。官方截至本次调研说明该旧名映射到当前 Flash 并按 Flash 价格计费。生产后台当前配置未读取，本次没有发起付费模型请求或核对生产账单。

## 外部计费依据

查阅日期：2026-11-12。来源均为 DeepSeek 官方文档：

- [模型与价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing)
- [Chat Completions 请求与 usage](https://api-docs.deepseek.com/zh-cn/api/create-chat-completion)
- [网页搜索与抓取计费规则](https://api-docs.deepseek.com/zh-cn/guides/search)
- [限速与 user_id 隔离](https://api-docs.deepseek.com/zh-cn/quick_start/rate_limit)

人民币单价，模型部分单位为每百万 tokens：

| 模型/工具 | 缓存命中输入 | 缓存未命中输入 | 输出 |
|---|---:|---:|---:|
| Flash，空闲时段 | 0.02 元 | 1 元 | 4 元 |
| Flash，高峰时段 | 0.04 元 | 2 元 | 8 元 |
| V4 Pro，空闲时段 | 0.15 元 | 4.5 元 | 13.5 元 |
| V4 Pro，高峰时段 | 0.30 元 | 9 元 | 27 元 |

高峰时段为北京时间工作日（不含中国法定节假日）09:00–12:00、14:00–18:00，其余为空闲时段。上线需核对供应商的时段归属规则及节假日来源；文档只明确时间范围，未明确跨边界请求如何选价。不能仅按周一到周五判定节假日。

Search、Fetch 各为每千次成功请求 5 元，即 0.005 元/次。只有 HTTP 200 且产生结果才计费：Search 的 results 非空，Fetch 返回网页正文；文档说明失败、无搜索结果或超时不计费。模型返回有效 usage 后，即使本项目判定生成内容不合格、拼写错误或写库失败，已产生的模型费用仍需入账。

模型费用 =（缓存命中输入 tokens × 命中单价 + 未命中输入 tokens × 未命中单价 + completion tokens × 输出单价）/ 1,000,000。

usage 提供 prompt_tokens、completion_tokens、prompt_cache_hit_tokens、prompt_cache_miss_tokens，以及 reasoning_tokens 明细。推理消耗按供应商 completion 口径结算，不把 reasoning_tokens 在 completion_tokens 之外重复相加。不可用 total_tokens 统一乘一个价格。

示例：Flash 高峰时段，某次调用有 2,000 个未命中输入 tokens、1,000 个输出 tokens，无缓存命中，费用约 0.012 元。5 元理论约可覆盖 416 次同等调用；此例不是实际查词次数承诺，未包含搜索、抓取、多次生成及重试。

## 建议产品规则

| 项目 | 建议行为 | 状态 |
|---|---|---|
| 普通用户默认额度 | 5.00 元/天，新老用户一致 | 建议，5 元是需求中的示例 |
| 管理员默认额度 | 不限额，仍统计使用金额；可配置有限额度 | “可不限额”已确认，可否给管理员主动设限待确认 |
| 每日边界 | Asia/Shanghai 当日 00:00 到次日 00:00，未用额度不累计 | 建议，与现有练习日期一致 |
| 额度取值 | null 表示不限额；0 表示暂停新增付费调用；正数表示每日金额 | 建议，不用 0 表示不限额 |
| 调整生效 | 保存后立即影响新请求；当天已用不清零 | 建议 |
| 降额 | 已用或已预留大于新额度时不撤销已发出的请求，停止后续调用 | 建议；调整时展示已用及预留提醒 |
| 用尽后 | 已有单词、缓存释义、已生成练习、普通背诵仍可用 | 建议 |
| 管理页面 | 每行显示日额度、今日已用、剩余；编辑金额并提供“不限额”选择 | 与需求一致 |
| 使用者提示 | 展示“今日 AI 额度不足，请明天再试或联系管理员调整”，必要时显示待结算金额 | 建议 |
| 计费范围 | 查词、补全、纠错、出题、模型复核、自动与手动重试；建议加上 Search/Fetch | 模型请求为需求范围，工具是否纳入待确认 |
| 豆包语音合成 | 本次模型额度默认不包含，可另行做语音或全部 AI 费用额度 | 建议，避免把不同服务混为同一账目 |

## 核心语义与数据模型

- 关键 ID：以数据库 users.id 为额度主体；请求中由登录态或后台任务的单词所有者确定，不信任前端传来的付费用户 ID。wordID、wordKey 是关联对象，不能替代用户 ID。
- 请求唯一 ID：每次真实供应商调用单独生成 request_id；用于预留及结算幂等。重试是另一条供应商请求，不复用一次结算记录冒充没有额外消耗。
- users：建议增加可空的每日人民币额度字段，使用金额必须使用整数精度或数据库 DECIMAL，不能使用 float64 累加。
- ai_daily_usage：建议按 (user_id, usage_date) 唯一记录当天 settled_amount、reserved_amount。以金额账本为准，每日换新日期行，保留历史，不依赖午夜清零定时器。
- ai_requests：建议持久化请求 ID、所属用户、业务入口、付费日期、供应商与模型、费率版本/时段、tokens、工具调用结果、预留/结算金额、状态和时间。保存实际响应模型及费率快照，调价不能回算修改旧费用。
- 金额精度：建议后端使用整数纳元（1 元 = 10^9 纳元）或等价精度 DECIMAL；页面额度输入精确到分，使用额显示至少四位小数，避免小额模型调用看起来始终为零。前端只能展示，不能计算权威余额。
- 多入口一致性：所有实际付费请求接入同一后端配额服务；缓存、只读轮询、答题判定不收费。现有查词函数缺用户参数；后台查词和每日生成必须显式传入受信任的用户身份或调用元信息。
- 跨用户共享缓存：由实际发出模型请求的用户承担费用，等待后直接复用完整缓存的其他用户不扣款。应先等同词锁、复查缓存，再申请额度；缓存修复不得污染其他用户数据。
- 管理员操作母库：按当前操作管理员记账。后台任务找不到原所有者或用户已删除/禁用时，停止新付费请求，不转嫁给其他用户。

## 实现方案与取舍

建议直接在现有 Go 后端增加配额与记账服务，复用 MySQL。当前项目有两套 chat 客户端和独立工具客户端，重点是统一控制，现阶段不需要引入外部代理网关或 Redis。

处理顺序：

1. 先判断是否可用缓存，并确认用户、模型、费率和请求大小上限。
2. 用短数据库事务锁定用户额度配置及当日使用行，检查已结算 + 已预留 + 本次预留是否超过上限；插入 request_id 及预留记录后提交。
3. 提交事务后再发供应商请求，数据库锁不能一直持有到模型返回。
4. 取得响应后先保存 usage/工具计费结果，再进行业务 JSON 校验；独立短事务把预留转换为实际费用，释放多余预留。同一个 request_id 只能结算一次。
5. 额度不足作为独立错误传播，后台任务结束本轮并解除查询状态，不进入现有普通失败重试。异步入口同时记录可供轮询展示的限额状态。

严格限额需要请求前预留“有依据的最大成本”，不能按平均费用预留后才发现超支。需要给查词、生成、复核显式设置 max_tokens，限制输入大小，并采用可靠的输入 token 上界及缓存未命中、高峰费率进行保守预留。思考模式会消耗输出 tokens，需显式确认模式和对应上限。无法证明上界、找不到模型价格或供应商规则未知时，不应宣称严格限额；有限额用户应暂停该调用并提示配置缺口。

出题含生成、复核及可选工具步骤。建议在工作流开始时预留足够完成一题的费用，且各个真实调用单独记录结算；失败或取消时结算已消耗部分、释放未发出部分。可防止生成花完剩余额度后无钱复核，导致用户花费却拿不到题。预留的取值及工作流 ID 细节留到后续计划确定。

请求已经发出后发生网络超时、响应损坏、进程中断，可能不知道模型实际 usage。不能一律退还预留当作零费用；应保留 uncertain/pending 记录及占用金额，通过可得的供应商记录或人工核对后结算。明确未发出的请求可释放。不能只根据预留记录过期就释放仍可能产生费用的调用。

若产品接受“已花到 5 元才禁止下一次请求”，实现更简单，但最后一个及并发请求会超出 5 元。因此本次推荐前置预留方案，必要时会在剩余金额为正、但不足完成请求时提前拒绝。

## 适用范围与改动落点

这是本项目账号级能力，不涉及某一客户的专属定制。预计影响用户模型/存储、schema 与幂等迁移、管理员接口及页面、查词/纠错后台链路、练习生成与工具客户端。

.claude/ai-profile.md 仍是模板路径，不能作为本项目真实公共层声明；按本次影响面判断，模型调用和账号存储属于项目内共享能力，后续修改必须保持已有调用方兼容，并避免改变词库数据语义。

现有新装 schema 与升级 Migrate 必须同步。仓库不存在 sql/upgrade/，已有升级机制在 backend/internal/storage/migrations.go，新增表/字段应复用该机制。本轮没有修改业务代码、配置、数据库或依赖。

## 影响面证据

以下为本次实际执行的 CodeGraph 输出；其 generateDailyWord 结果仅列出直接方法及测试，调用链仍需结合已读 daily.go:232 和 training_worker.go:20 理解，不能据此漏掉每日队列。

```text
Impact of changing "lookupDeepSeek" — 5 affected symbols:

backend/internal/app/deepseek.go
  function    lookupDeepSeek:37
backend/internal/app/translate.go
  function    translateWord:19
backend/internal/app/dictionary.go
  method      handleRetryDictionary:114
backend/internal/app/words.go
  method      translateAndSave:186
backend/internal/app/correction.go
  method      handleCorrectWord:61

Impact of changing "getDeepSeekConfig" — 11 affected symbols:

backend/internal/app/settings.go
  method      getDeepSeekConfig:153
  method      maskedSettingsView:228
  method      handleGetSettings:249
  method      handleUpdateSettings:267
backend/internal/app/dictionary.go
  method      handleRetryDictionary:114
backend/internal/app/words.go
  method      translateAndSave:186
  method      spawnTranslation:116
backend/internal/app/correction.go
  method      handleCorrectWord:61
backend/internal/app/training_worker.go
  method      generateDailyWord:20
backend/internal/app/training_worker_test.go
  function    TestDailyDisabledProviderReturnsClearFailure:12
  function    TestLiveDailyGenerator:22

Impact of changing "generateDailyWord" — 3 affected symbols:

backend/internal/app/training_worker.go
  method      generateDailyWord:20
backend/internal/app/training_worker_test.go
  function    TestDailyDisabledProviderReturnsClearFailure:12
  function    TestLiveDailyGenerator:22

Impact of changing "Call" — 8 affected symbols:

backend/internal/training/provider.go
  method      Call:33
backend/internal/training/provider_test.go
  function    TestProviderPreservesProtocolAndNullableUsage:14
  function    TestProviderClassifiesRemoteErrors:39
  function    TestProviderTimeoutEndsRequest:66
backend/internal/training/live_test.go
  function    TestTrainingLiveProvider:12
backend/internal/app/training_worker.go
  method      generateDailyWord:20
backend/internal/app/training_worker_test.go
  function    TestDailyDisabledProviderReturnsClearFailure:12
  function    TestLiveDailyGenerator:22
```

## 边界行为清单

- 空/无数据：新用户和当日无请求时已用为 0；不限额显示“不限额”，剩余不伪造数字。
- 异常返回：额度不足与远端失败区分；数据库故障不得绕过记账继续付费；业务内容失败但有 usage 仍结算。
- 历史兼容：普通老用户建议补默认额度，管理员补不限额；历史消费无 usage 无法准确回填，生效前消耗不伪造。上线当日按启用后的记录统计，或选择次日开始完整自然日。
- 本地/mock：仅使用模拟响应验证规则，模拟调用不进入真实生产费用；不得用浏览器 localStorage 作为配额存储。
- 契约：建议用户列表增加额度及今日使用字段，新增仅管理员可调用的单用户额度修改接口，增加当前用户自己的额度查询；同步登录态类型及错误展示。具体字段/路径在后续计划确定，本轮没有新增实际接口。
- 权限：普通用户不能改自己或别人的额度；管理员是否主动设限遵循配置语义，不无条件绕过有限额度；禁用和删除优先于不限额。
- 并发与幂等：不同单词、不同标签页、查词与出题、后台重试共享一个日余额；数据库事务防止并发超发；网络重投不能重复结算。
- 重启：日额度及未结算请求持久化；现有练习内存队列重建后仍使用此前账本。
- 日切：每个实际请求以获得预留时的上海日期归属；午夜前开始、午夜后返回的费用仍结算旧日。工作流中午夜后新增的供应商请求按新日重新预留，不能沿用旧日预留发新请求。
- 调额：保留当天账本，不重置；已经发出的调用继续结算，尚未发出的后续步骤重新检查额度与账号状态。
- 费率：明确供应商/BaseURL、响应模型别名、费率版本和生效期；未核对的新模型对有限额用户阻断，对不限额用户标记费用未知，不能显示为免费。

## 验收标准（后续实现使用，本轮未执行）

- [ ] 普通用户默认额度、管理员不限额及自定义额度按最终确认规则展示，普通用户直接调用管理接口也无法修改。
- [ ] 今日已用按输入缓存、输出、已确认的工具计费和费率快照计算；生成内容被拒绝也不丢费用；不限额管理员同样有统计。
- [ ] 缓存命中及普通背诵可继续；查词、纠错、出题、复核、工具和重试都不能绕过限额。
- [ ] 多请求并发下已用加预留不突破配置额度；只剩不足完成请求的钱时清楚说明额度不足。
- [ ] 加额度立即允许新调用，降额度不清空已用；0 元阻止新付费调用，不限额正常使用。
- [ ] 上海午夜换日、跨午夜结算、服务重启及重复结算均正确；未知费用不会自动退款后再次放行。
- [ ] 异步查词被限额后不永久显示“查询中”，不反复自动调用；额度提升后提供明确的重试路径。

## 依赖服务与可复用资产

| 服务/方法 | 当前签名或资产 | 满足情况/变更点 |
|---|---|---|
| DeepSeek Chat Completions | POST /chat/completions；usage 与 max_tokens | 可支持用量结算与输出上限，需补解析、费率管理及预留 |
| DeepSeek Search/Fetch | POST /v1/tools/search、/v1/tools/fetch | 固定成功调用价格；目前未统一计费 |
| lookupDeepSeek | (ctx, word, cfg, reason...) → ([]Sense, bool, error) | 缺用户及 usage，需要接入受控调用 |
| training.Provider.Call | (ctx, system, data) → (string, modelUsage, error) | 可复用 nullable tokens，但缓存明细及异常 usage 保留不足 |
| currentUser / User.ID | 登录态用户身份及个人词库所有者 | 可复用，不接受客户端替换计费主体 |
| UserRepository / UserStore | 用户 CRUD、List 等 | 增加额度读写，用户列表合并当日账本 |
| storage.Migrate / schema.sql | 现有幂等升级与新装结构 | 新增字段和表，不引入第二套升级机制 |
| DailyManager.day | 上海日期与用户队列隔离 | 日期逻辑可复用；内存队列不作为费用记录 |

## 后续需要确定的事项

1. “每天 5 元”是否包含 DeepSeek Search/Fetch。建议包含，这样更接近该账号实际产生的 DeepSeek 支出；豆包 TTS 本次单列。
2. 管理员是默认不限额但允许设限，还是所有管理员永远豁免；建议默认不限额且可编辑。
3. 最终默认金额是否采用 5 元，新功能在上线当日即时生效还是次日零点生效。
4. 实现前核对当前生产 BaseURL/模型、费率时段归属、节假日及可用于核对未知 usage 的账单能力。供应商自定义网关需按其真实价格，不能套用 DeepSeek 官网价格。

本次证据包括当前源码、数据库结构、CodeGraph 影响面和官方文档。没有做生产扣费验证或业务验收。计费与并发规则适合后续用模拟供应商和数据库事务验证；界面验收按浩原要求交由手动操作。

实现补充（2026-11-12）：按 02b 落地；供应商费率边界采用较高费率保守结算，避免请求跨时段低估。未知价格在事务内再次检查有限额状态，避免管理员调额与请求并发时绕过。普通账号与新管理员默认额度在首次升级设置后不再重置。

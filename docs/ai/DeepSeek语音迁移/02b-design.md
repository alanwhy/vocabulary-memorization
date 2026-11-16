# DeepSeek 语音详细设计

任务身份、阶段状态与授权以 [01-spec.md](01-spec.md) 为准。本文件为设计，不代表已实现。阶段：plan；GPT-6；2026-11-16（Asia/Shanghai）；`@sw/dev-skills@2.0.0`。

## 接口契约

| 方法 | 路径 | 用途 | 提供方/调用方 |
|---|---|---|---|
| GET（保留） | `/api/pronounce/{wordKey}` | 返回共享单词音频，未命中生成并保存 | Go app / speakWord |
| POST（新增） | `/api/pronounce/example` | 按需合成例句，仅返回音频 | Go app / speakExample |
| GET/PUT（字段变化） | `/api/admin/settings` | 既有 DeepSeek 配置及 TTS 音色 | Go app / AdminSettings |

### 单词朗读

请求示例：`GET /api/pronounce/well-known`，携带现有登录 Bearer token。URL 由前端 encodeURIComponent 编码。先 TrimSpace、转小写，再用现有 wordPattern 校验；不会把任意句子当成单词持久保存。

成功：200，正文为完整音频二进制。旧 MP3 返回 `Content-Type: audio/mpeg`；新 WAV 返回 `Content-Type: audio/wav`。不将 WAV 写入 `.mp3`，不要求前端转码。缓存共享的是标准化词键，不要求该账号已经录入词条，保持查词弹层可点读行为。

### 例句朗读

请求示例：

```json
{"text":"She recorded the new words in her notebook."}
```

JSON text 必填、首尾空白去除后非空；保留内部大小写、空格和标点。前端直接传 `s.example`，不传中文翻译或 HTML。应用限制 1,000 个计费字符（汉字 2，其余 Unicode 字符 1）、请求体 8 KiB，低于官方 10,000 字符/64 KiB 上限。实现时同时约束请求体大小和内容长度，不依赖前端校验。

成功：200，完整 WAV 二进制，`Content-Type: audio/wav`、`Cache-Control: no-store`。POST Body 包含原文，不把例句放进 GET URL。服务端不调用任何文件保存函数，不将内容或音频写数据库，也不在完成后保留内存缓存。请求期间的必要缓冲在返回后释放。

### 错误映射

所有失败返回现有 JSON 结构，如 `{"error":"语音合成未配置，请在后台配置 DeepSeek"}`。完整读取及校验音频后才向前端写成功头，避免已发 200 后才报告供应商失败。

| 状态码 | 条件 | 前端表现 |
|---|---|---|
| 400 | 非法词键、JSON 错误、空例句、无有效可朗读内容 | 提示输入/内容错误，不请求或不重试模型 |
| 401/403 | 沿用现有 requireAuth/requireAdmin | 会话过期或无权限提示；无供应商调用 |
| 413 | 例句请求体或计费字符超过应用限制 | 提示例句过长 |
| 429 | 供应商并发/限流 | 提示稍后重试，不自动重试 |
| 503 | 凭证缺失、语音服务忙、关闭期间拒绝新任务 | 明确配置或繁忙提示 |
| 504 | 等待/供应商超时 | 明确超时提示，用户可重新发起 |
| 502 | 供应商鉴权失败、其他上游错误、非音频/空音频、读响应中断 | 提示语音服务失败；不把供应商 401 映射为本系统登录失效 |
| 500 | 缓存读取权限/磁盘错误、单词保存失败 | 提示存储失败；不伪装成缓存未命中 |

用户取消不产生错误 toast；过期响应不再改变 UI。错误提示不直接透传供应商原始正文或凭证。

### 配置契约

既有 `enabled/api_key/base_url/fallback_model/thinking_model/start_color/end_color/segments` 保持现有语义与密钥掩码规则。增加 `tts_voice_id`，仅允许 `stella` 或 `tide`，默认 stella；GET 返回该字段，PUT 无该字段时保留当前值，有非法值返回 400。

删除 `tts_api_key`、`tts_cluster`、`tts_voice_type` 的 GET/PUT 字段及对应播种/迁移/读取逻辑。旧前端提交这些字段不作为可用 TTS 配置；本次不新增旧页面适配层。旧 settings 行无需删表/删行，但运行时不再读取。

语音复用 `deepseek_api_key`、`deepseek_base_url`。getTTSConfig 在同一个 settingsMu 读取快照中返回 APIKey/BaseURL/VoiceID，不继承查词 Enabled 条件、模型名或 Accounting。空/掩码 Key 提交仍按既有规则保留真实 Key。

新增数据库配置键 `deepseek_tts_voice_id`，首次缺失时从 `DEEPSEEK_TTS_VOICE_ID` 或默认 stella 播种；环境变量仅首次播种，后续以后台设置为准。无新表、索引或 schema 升级。

### 外部 DeepSeek 契约

向配置 Base URL 拼接 `/v1/tools/tts`；默认 `https://api.deepseek.com/v1/tools/tts`。尾斜杠去除；配置末尾已有 `/v1` 时仅拼接 `/tools/tts`，不得出现 `/v1/v1`。其他自定义网关前缀按实现联调核对。

```json
{"text":"apple","voice_id":"stella","format":"wav","filter_markdown":true}
```

鉴权为 DeepSeek API Key 的 Bearer，Content-Type application/json。不传聊天 `model`、豆包 cluster、speed_ratio 或 X-Api-Key，不解析 JSON/base64 成功数据。HTTP 接口足够满足短词/例句，不引入 WebSocket。

## 后端流程与存储

### 单词持久缓存

1. 校验规范词键，读取原位旧 MP3；有效命中直接返回，包括 API Key 未配置时。
2. 再读 `AUDIO_DIR/deepseek/<sha256(word_key)>.wav`；命中直接返回。缓存键不包含音色或用户，保障既有内容持续复用。
3. 两者均未命中时，进入 App 的语音任务表，按 word_key 合并等待者。取得执行权后再次检查两种文件，防止等待期间其他入口已生成。
4. 检查配置；在同一目标目录准备临时文件，以尽量提前发现不可写目录。再通过独立 TTS 并发信号量调用供应商。
5. 完整读完有大小上限的响应，确认非空及基本 WAV 结构；使用供应商真实流式 WAV 约定，不在未验证情况下要求其声明长度必须等于文件长度。读错、超限或明显非音频时返回失败。
6. 临时文件写完、关闭成功后，同目录原子发布为最终 `.wav`；旧 `.mp3` 永不写入。发布前如已出现有效缓存，复用它并清理本任务临时文件。
7. 仅保存成功后发布结果给等待者，再移除任务表项。失败只清理本任务临时文件，不删旧 MP3 或已完成 WAV，不持久缓存失败结果。

TTS 任务表使用完成通知和短暂结果，不使用永久增长的按词锁 map。后台预生成和 GET 共享这一入口。后台 `spawnTTS` 保持异步触发，不阻塞释义保存。

单词任务挂在 App 后台 context 上，前端取消仅退出该等待方；已发起的同词生成继续完成并保存，避免快速切换导致多次收费。单词生成无自动重试；网络未知失败可能已经产生供应商费用，不承诺跨失败/跨进程恰好一次。

### 例句临时合成

校验 → 读取同一 TTS 配置 → 通过并发限额合成 → 完整读取 WAV → no-store 返回。使用请求 context，取消或超时停止本次上游读取。不得进入单词生成任务表、落盘函数或完成音频缓存。

### 超时、并发及关闭

- 独立 TTS 并发上限默认 5，单词/例句共同占用，不占用正在执行的查词名额；官方账号上限为 50，但本项目先采用更低值。
- TTS 上游总预算建议 20 秒；两个语音路由单独预算 25 秒，保留其他路由 10 秒。当前 HTTP Server WriteTimeout 为 30 秒，初版不扩大它。真实延迟验收若需要变更预算，同步调整本设计。
- 等待重复词任务受请求 context 控制，停止期间拒绝新任务。
- 将语音任务纳入 CancelBackground/WaitBackground；开始新任务、标记 stopping 与 WaitGroup.Add 的顺序受锁保护，避免关闭时 Add/Wait 竞争。
- 语音响应设置合理大小上限，初版建议 16 MiB；这只保护内存，不能将超限或中断的内容当成功缓存。

## 前端设计

### 组件与调用关系

| 文件/组件 | 输入与动作 | 职责 |
|---|---|---|
| `usePronunciation.js` | speakWord(wordKey, options)、speakExample(text, options)、stopSpeech(sourceId) | Bearer 请求、业务错误提示、暴露共享播放状态 |
| `utils/audioPlayback.js`（新建） | 注入 fetch/Audio/URL，目标键、来源 ID、URL/请求体 | 控制唯一活动请求/音频、乱序响应及 URL 清理；不依赖 Vue/别名以便 Node 测试 |
| WordCard | 单词、每条英文例句 | 单词按钮保留；英文例句旁加 type=button 朗读，click.stop |
| FlashcardView | 当前单词、frontExample、各 sense.example | 正反面例句按钮；切词/离页停止旧来源；保留切词自动读词、V 键和结果汇总 |
| WordLookupTooltip | 当前查询 wordKey、各 sense.example | 例句按钮；关闭/换词仅停止属于弹层的播放 |
| AdminSettings | tts_voice_id | 用音色下拉替代豆包三字段，说明共用 DeepSeek 凭证 |

### 状态与资源

全局共享状态为 idle/loading/playing、当前目标键、来源 ID、递增请求序号、AbortController、活动 Audio、Object URL。句子原文只是当前请求/播放器必要状态，结束后清空，不作为结果缓存。

- 同目标仍 loading 时忽略再次点击；播放结束后再次点击例句产生新 POST。
- 新目标到来立即停止前一播放器、撤销其 URL，取消旧 fetch，并增加请求序号。
- 任意 await 完成后核对序号及来源有效性；旧响应不得播放或覆盖新状态。
- ended/error/替换/关闭所属视图都释放 URL 和活动资源；清理绑定来源，弹层关闭不能误停刚刚从闪卡发起的新声音。
- 控件请求中显示状态并避免重复提交；aria-label 为“朗读例句”，不把“DeepSeek/API/缓存”等实现细节塞进例句用户流程。
- 单词自动播放 NotAllowedError 沿用现有静默处理；手动播放失败保留明确提示。
- 不新增慢速控件，不改变 token 分词、词高亮和点击查词。闪卡朗读按钮始终 stopPropagation，不导致翻面。

项目使用 JavaScript/Vue SFC，不新增 TypeScript 改造；契约由 JSON 字段、JSDoc 和音色枚举表达。

## 必须覆盖的测试

| 对象 | 覆盖点 | 当前基础/计划 |
|---|---|---|
| DeepSeek TTS 适配器 | URL 拼接、Bearer/JSON、WAV 成功、429/503/超时/非音频/中断/超限 | 新 `deepseek_tts_test.go`，httptest，不访问真实模型 |
| 单词缓存 | 旧 MP3 字节/mtime 不变、优先级、WAV 持久化、重复/跨入口命中、路径与磁盘错误 | 新 `pronunciation_test.go`，临时目录，不删真实音频 |
| 同词生成 | 多请求+后台合成仅一次、等待方取消后其他方成功、任务完成移除、关闭无竞态 | pronunciation 测试 + go test -race |
| 例句 API | 鉴权、全文、不含译文、空/超限、no-store、无文件/数据库保存、取消 | pronunciation/routes 测试 |
| 设置 | 新音色枚举/默认/保存、共用 Key、掩码保留、旧豆包 Key 不迁移、关闭查词不禁读音 | 扩展 settings_test.go，以临时 settings fake 验证 |
| 前端播放核心 | A 慢 B 快只播 B、同目标加载防重、例句结束后重播新请求、取消不 toast、旧 URL/Audio 释放 | 新 audioPlayback.test.js，Node 内置 node:test |
| 实际 UI/发音 | 五处英文例句、自动读词、V 键、结果点读、弹层、移动布局、发音质量 | 浩原手动验收，或获实现授权后使用 ego-lite 验证可操作行为 |

## 契约变更记录

| 日期 | 变更 | 来源 | 同步状态 |
|---|---|---|---|
| 2026-11-16 | 保留历史 MP3，新增 DeepSeek 单词 WAV、临时例句 POST、tts_voice_id；删除豆包配置字段 | 浩原迁移与规划指令 | 本仓库前后端在本设计共同约定，已实现，自动验证完成；真实发音与 UI 待人工验收 |

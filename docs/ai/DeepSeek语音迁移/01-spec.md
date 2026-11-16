---
task_id: 25fc711b-7514-4ac7-bf48-59846db33098
status: awaiting_user
current_stage: impl
stage_status: ready
execution_mode: staged
updated_at: 2026-11-16T08:31:47+08:00
review_base: 0ba6f6ffd2c7d566198e3aa8741970e591df2172
---

# DeepSeek 语音迁移与例句朗读

执行模式：本次仅需求分析、详细设计及实现计划，禁止进入编码。来源：浩原 2026-11-16 明确要求“规划……不要实现代码”。该指令授权完整规划，不需要在 spec 与 plan 之间重复确认。后续实现须获得新的实现指令。

2026-11-16 阶段确认：浩原显式调用 sw-impl，授权按本计划编码与验证，取代上述规划阶段的“不实现代码”限制；不授权提交、推送、部署或飞书归档。按项目要求不启动独立复核，实际听感与页面体验交由用户手动验收。起始工作区：`.env.example` 仅既有末尾换行修改；`frontend/pnpm-lock.yaml` 未跟踪；本任务规划文档未跟踪，均保留。

阶段记录：GPT-6（具体模型变体未暴露）；文档整理 2026-11-16 07:45:24–07:49:48，Asia/Shanghai；规范 `@sw/dev-skills@2.0.0`。代码与影响面调研先于文档整理完成。

## 需求陈述

将新增语音合成从豆包切换为 DeepSeek 官方 TTS，并增加英文例句朗读。单词音频生成成功后持久保存，重复播放复用音频，避免重复请求供应商。例句音频按需生成、播放后释放，不持久保存。

浩原已确认的约束：

- 移除豆包供应商调用、配置界面、协议解析及相应实现；不保留供应商切换或豆包回退。
- 已由豆包生成的音频不得删除或覆盖，继续播放。
- 新生成的单词音频必须保存；不能把一次成功合成但保存失败视为持久缓存成功。
- 例句音频不落盘、不入数据库、不进入浏览器长期缓存。
- 规划阶段只写文档；浩原随后显式调用 sw-impl 授权编码与验证。本次实现不执行发布，不读取或改动生产音频卷及数据库。

## 适用范围与改动落点

这是整个背单词产品的能力迁移，包含 Go 应用层、Vue 单词/例句展示入口、后台语音设置及使用文档，不是单客户定制。

例句指现有释义的 `Sense.example` 英文原文：单词列表（首页及归档页复用 WordCard）、闪卡正面首条例句、闪卡背面各词性例句、查词弹层各词性例句。中文译文 `example_translation` 不朗读。同义替换训练的题干、选项不是本次释义例句入口。

单词朗读保留现有入口和行为，包括闪卡切词自动朗读、正反面喇叭、结果汇总点读及 V 快捷键。例句仅手动点击，不增加自动朗读或新快捷键。

项目 `.claude/ai-profile.md` 为旧模板，公共层示例未获得确认；按实际调用影响面判断。共用播放工具是本项目多个入口的产品共用能力，需覆盖调用方兼容性，不改变跨项目公共 SDK、释义生成、分词高亮和复习规则。

通用前置脚本返回 `AGENTS.md: legacy-needs-confirmation`，未迁移该文件；保留当前项目指令，不影响本次文档规划。

## 现状证据与影响面

以下为本次 CodeGraph 和配置/文档检查结果，未读取生产后台配置或生产音频卷。

| 现有落点 | 已确认事实 | 迁移影响 |
|---|---|---|
| `backend/internal/app/doubao.go:78` | 调用火山 `/api/v1/tts`，X-Api-Key 鉴权，JSON/base64 响应，请求 MP3 | 整个适配器及其测试替换为 DeepSeek HTTP 音频响应 |
| `backend/internal/app/pronunciation.go:17,70` | `audio/<word_key>.mp3`；GET 缓存命中即返回；未命中合成并写盘 | 兼容旧 MP3，新增 WAV 存储；统一生成/缓存入口 |
| `backend/internal/app/pronunciation.go:51` | 后台预生成不复查已有文件，直接合成并覆盖写入；写盘失败仅日志 | 必须保护旧文件、消除重复合成和半文件可见性 |
| `backend/internal/app/words.go:245` | 查词成功后调用 `spawnTTS` | 保留触发时机，改用统一 DeepSeek 单词生成流程 |
| `backend/internal/app/app.go:106`、`lifecycle.go` | App 管理后台 context、并发和 WaitGroup | 新增语音生成去重/并发状态，纳入关闭管理 |
| `backend/internal/app/settings.go` | 豆包 Key/Cluster/VoiceType 独立配置；DeepSeek Key/Base URL 已存在 | 移除豆包配置读写，复用 DeepSeek 凭证，增加官方音色枚举 |
| `frontend/src/composables/usePronunciation.js:8` | 各入口统一调用 speakWord；请求完成后才停止旧音频；Object URL 未释放 | 共用单词/例句播放，阻止乱序响应串音，释放临时资源 |
| `WordCard.vue`、`FlashcardView.vue`、`WordLookupTooltip.vue` | 显示英文原文及中文译文，没有例句朗读按钮 | 在现有英文例句旁增加按钮，保留高亮/查词/翻卡行为 |
| `docker-compose.yml:43`、`docker-compose.production.yml:9` | `/app/audio` 挂载既有 `vocabulary-memorization_audio_data` 外部卷 | 复用原卷；禁止删除卷或通过改名创建空卷 |

CodeGraph `impact` 原始输出：

```text
Impact of changing "synthesizeSpeech" — 4 affected symbols:
backend/internal/app/doubao.go
  function    synthesizeSpeech:78
backend/internal/app/pronunciation.go
  method      synthesizeAndSave:51
  method      spawnTTS:28
  method      handlePronounce:70

Impact of changing "getTTSConfig" — 7 affected symbols:
backend/internal/app/settings.go
  method      getTTSConfig:163
  method      maskedSettingsView:232
  method      handleGetSettings:253
  method      handleUpdateSettings:271
backend/internal/app/pronunciation.go
  method      synthesizeAndSave:51
  method      spawnTTS:28
  method      handlePronounce:70

Impact of changing "spawnTTS" — 3 affected symbols:
backend/internal/app/pronunciation.go
  method      spawnTTS:28
backend/internal/app/words.go
  method      translateAndSave:187
  method      spawnTranslation:117

Impact of changing "speakWord" — 12 affected symbols:
frontend/src/composables/usePronunciation.js
  function    speakWord:8
frontend/src/components/word/WordLookupTooltip.vue
  component   WordLookupTooltip:1
  file        WordLookupTooltip.vue:1
frontend/src/App.vue
  component   App:1
  file        App.vue:1
frontend/src/components/word/WordCard.vue
  component   WordCard:1
  file        WordCard.vue:1
frontend/src/components/word/WordList.vue
  component   WordList:1
  file        WordList.vue:1
frontend/src/views/FlashcardView.vue
  file        FlashcardView.vue:1
  function    onShortcut:135
  component   FlashcardView:1
```

## 核心语义与数据模型

- **已确认：关键 ID。** `word_key` 是录入英文经 `TrimSpace`、转小写后的键，包含合法空格、连字符、单引号；现有 `wordPattern` 限制 1–64 字符。继续以这个键共享音频，不以用户词条 ID 或显示大小写建缓存。内部空格不自行重新折叠，防止改变既有键。
- **设计：存储。** 旧文件保持 `AUDIO_DIR/<word_key>.mp3` 原位。新单词 WAV 存为 `AUDIO_DIR/deepseek/<SHA-256(word_key)>.wav`。目录仍位于原持久卷，不新建音频表。缓存命中不依赖当前 API Key 是否已配置。
- **设计：缓存优先级。** 有效旧 MP3 → 已保存的新 WAV → 单次 DeepSeek 合成并原子保存。后台预生成和点击播放都使用同一流程，不覆盖已有有效文件。
- **设计：音色变化。** 默认女声 `stella`，可选男声 `tide`。已有音频继续复用，切换音色只影响后续尚未生成的单词及新例句，不自动清空缓存。
- **设计：例句。** 使用完整 `Sense.example` 原始英文文本，保留大小写/标点；不拼接中文译文，不从高亮 DOM 提取文字。一次播放仅短暂保留网络响应和 Blob，结束、切换或离开后释放。重复点击已结束的例句会再次请求，这是不保存音频的预期行为。
- **设计：多入口一致性。** 单词和例句在全页面共享一个活动请求/播放器，新目标取代旧目标；同目标请求未完成时重复点击不发第二次请求。
- **已确认：既有费用范围。** `docs/ai/用户每日模型费用额度/01-spec.md` 的用户确认记录明确“包含 Search/Fetch、不含语音”。本次保持这一范围，不将 TTS 新增到用户日额度账本。TTS 仍会产生供应商费用，应在使用文档说明。

## 设计要点

1. 新增单一 DeepSeek TTS 适配器，不加入供应商工厂、双服务切换或回退；旧 MP3 读取是历史数据兼容，不依赖豆包服务。
2. 共用既有 DeepSeek API Key/Base URL；语音设置只保留 `tts_voice_id`。不将豆包 Key 当成 DeepSeek Key 迁移。现有“启用 DeepSeek 查词”开关仍仅控制查词，不控制已有音频或语音功能。
3. 新单词生成需经过同词任务去重和二次缓存检查；合成先完整读完响应，再原子写入。磁盘不可写时尽量在调用供应商之前发现并返回错误。
4. 例句走独立 POST 接口，不调用单词保存流程；响应 `Cache-Control: no-store`。不建立服务端完成结果缓存、Service Worker 缓存、IndexedDB/localStorage 音频缓存。
5. 不增加语速、英美口音、ASR、发音评分或例句预生成。接口、错误码、任务生命周期和播放状态见 [02b-design.md](02b-design.md)。

## 边界行为清单

| 边界 | 规划行为 |
|---|---|
| 空单词/非法路径/超长词 | 复用现有录入合法性校验，400；不读盘越界，不请求模型 |
| 空英文例句、只有中文译文 | 无朗读按钮；直接 API 请求空白文本返回 400 |
| 超长例句 | 本应用上限 1,000 个计费字符、请求体 8 KiB；超限 413，无付费调用 |
| 旧 MP3 与新 WAV 同时存在 | 旧有效音频优先，两个文件均不删除、不覆盖 |
| 缓存文件不存在/零字节 | 视为未命中；零字节旧文件保留原位，新结果写 WAV；仅 ENOENT/明确坏缓存进入生成，权限/磁盘读取错误不得当缺失反复收费 |
| 新 WAV 空响应/明显无音频/响应中断 | 合成失败，不发布为正式缓存、不返回成功音频；清理本任务临时文件 |
| 未配置凭证 | 已有音频照常播放；新单词及例句返回 503 并提示配置 DeepSeek |
| 供应商 429、超时、网络错误 | 明确错误；不自动循环重试、不回退豆包；用户可手动重试 |
| 单词落盘失败 | 500 明确提示保存失败，不能返回“已持久缓存”成功；已有音频不受影响 |
| 并发单词、后台与点击竞争 | 单进程内同 word_key 共用任务，最多一次上游合成；等待方取消不破坏其他等待方 |
| 前端切换单词、例句、关闭所属弹层 | 停止所属旧播放器，忽略过期响应，释放 URL；单词已启动生成可继续保存，例句取消后不继续留存 |
| 浏览器拦截自动播放 | 保留现有策略，闪卡自动朗读被拦截不弹发音失败；用户可点喇叭 |
| 角色与账号 | 两种朗读接口均沿用 Bearer requireAuth；语音设置 requireAdmin；词音频全局共用，例句不形成跨账号持久数据 |
| mock 与本地文件 | Go 测试使用 httptest/临时音频目录/假仓储，不能误写真实 `audio/`；前端测试使用注入的 fetch/Audio/URL 替身 |
| 旧配置兼容 | 移除豆包配置的播种、读取、保存及展示，不保留旧前端参数兼容层；历史 settings 行不再使用，不为此执行数据库删除 |
| 契约同步 | 保留单词 URL，响应 Content-Type 支持 MP3/WAV；新增例句 POST 和音色枚举，前后端同次交付；说明文档同步 |

## 验收标准

- [ ] 不配置 DeepSeek 凭证仍可播放已生成的旧豆包音频，文件字节、路径和修改时间不变。
- [ ] 新单词只调用 DeepSeek，生成完整 WAV 并保存；刷新、重启服务、其他账号和其他入口再次播放不重复合成。
- [ ] 并发点击及与后台预生成同时发生时，同词最多调用一次上游；缓存不会被覆盖为半文件。
- [ ] 首页/归档单词卡、闪卡正反面、查词弹层每条非空英文例句可手动朗读正确完整原文。
- [ ] 例句播放前后音频目录和数据库没有新增例句音频；响应 no-store；结束/切换后 Blob URL 被释放；再次播放产生新请求。
- [ ] 例句喇叭不触发闪卡翻面、不打开查词弹层，不改变例句高亮、中文译文及既有 V 键动作。
- [ ] 快速切换目标时只播放最后目标，取消/过期响应不误报，不积累活动音频资源。
- [ ] 后台不再出现豆包 Key/Cluster/VoiceType，业务代码不再包含豆包请求协议或回退分支；旧 MP3 保留读取能力。
- [ ] API 鉴权、非法输入、供应商错误、读写失败、取消及关闭场景有针对性测试；实际英文发音质量和浏览器体验由浩原手动验收。

## 依赖服务现状与可复用资产

官方资料查阅于本会话 2026-11-16：[发布公告](https://api-docs.deepseek.com/zh-cn/news/news261113/)、[TTS 指南](https://api-docs.deepseek.com/zh-cn/guides/tts/)、[TTS API](https://api-docs.deepseek.com/zh-cn/api/tts/)、[价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/)。11 月 13 日正式开放；HTTP POST 返回流式音频，WAV/PCM；stella/tide；英语等 10 种语言自动识别；无语速或英美口音参数。当前折扣价每万计费字符 1.5 元，汉字算 2，其他字符算 1。

| 服务/资产 | 签名或用途 | 复用/变更 |
|---|---|---|
| DeepSeek TTS | `POST /v1/tools/tts`，Bearer，JSON text/voice_id/format/filter_markdown | 新建专用客户端，不复用 Chat Completions 响应解析 |
| synthesizeSpeech | `(context.Context, string, ttsConfig) ([]byte, error)` | 改为 DeepSeek WAV 合成语义，删除旧协议解析 |
| getTTSConfig | `() ttsConfig` | 从同一配置快照取得 DeepSeek 凭证及 voice_id，不注入聊天计费回调 |
| AUDIO_DIR 和生产音频卷 | 共享单词持久存储 | 原位复用，仅增加新 WAV 子目录 |
| settings 表/SeedIfMissing/UpsertMany | 配置持久保存 | 只增加音色配置键，不新增表或数据库 schema |
| WordCard、闪卡、弹层、speakWord | 现有词音/例句入口 | 保留入口并扩展例句按钮与共用播放状态 |

## 实施前置与本次未验证

- 生产 DeepSeek 凭证、Base URL、余额及网关是否支持 `/v1/tools/tts` 未检查。若使用自定义网关，需在实现联调时核对实际接口；不能假定兼容聊天接口就兼容 TTS。
- 未发起付费 TTS 请求，未试听英语发音，未读取/迁移生产音频卷。
- 当前部署是单后端实例；本计划去重保证限于同一进程。扩为多实例需要另行设计跨实例锁，不宣称全局恰好一次。
- 本次工作区已有 `.env.example` 修改及未跟踪 `frontend/pnpm-lock.yaml`，均属既有内容，保持原状。
- 没有需要再次向用户确认的需求取舍。默认女声、共享凭证、例句手动朗读及不扩大费用账本为当前建议方案，后续可按用户反馈调整文档。

## 实现交付状态

2026-11-16 08:30（Asia/Shanghai）：已按 sw-impl 完成实现、自动验证与交付记录。计划项进度见 02-plan.md，实际命令/结果与待人工验收见 03-review.md；未独立复核、未提交推送、未部署、未归档。真实语音服务试听与 UI 验收仍待浩原完成。

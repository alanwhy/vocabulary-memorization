# DeepSeek 语音迁移实现计划

任务身份、状态与授权以 [01-spec.md](01-spec.md) 为准，接口与数据流以 [02b-design.md](02b-design.md) 为准。阶段：plan；GPT-6；2026-11-16（Asia/Shanghai）；`@sw/dev-skills@2.0.0`。

本次是语音供应商替换、历史音频兼容与例句朗读新功能。规划阶段不实现代码；2026-11-16 浩原显式调用 sw-impl 后已完成下列实现项。实际验证见 03-review.md，真实 UI/音质仍待手动验收。

## 验证方式

### 验证前置

| 命令/操作路径 | 预期结果 |
|---|---|
| 后端目录执行 `go test ./internal/app -run 'TestDeepSeekTTS\|TestPronounce\|TestTTS\|TestSettings\|TestHandlerRegistersExistingAPIRoutes'` | 适配器、缓存、设置、鉴权及异常场景通过；上游请求数符合去重断言 |
| 后端目录执行 `go test -race ./internal/app -run 'TestPronounce\|TestTTS'` | 同词并发、取消和关闭无数据竞争，无重复上游调用 |
| 后端目录执行 `go vet ./...`、`go test ./...` | 不影响查词、词库、闪卡、训练、用户费用功能；不能替代真实发音试听 |
| 前端目录执行 `node --test src/utils/audioPlayback.test.js` | 请求乱序、加载防重、临时资源释放、重播例句重新请求通过 |
| 前端目录执行 `npm run lint`、`npm run build` | 现有 lint/build 入口通过；lint 带 fix，执行后核对修改范围 |
| 测试目录放已有旧 MP3，记录哈希/mtime；不配置 TTS Key，点击单词；再触发同词后台预生成 | 正确播放，供应商请求为 0，旧文件哈希/mtime 不变 |
| 新词第一次点击/查词预生成后，再在闪卡、弹层、其他账号播放，并重启测试服务重复播放 | 一次合成并保存完整 WAV，之后均读文件；跨服务重启仍命中 |
| 同一新词多个 GET 与后台预生成同时发起 | 模型只调用一次，等待者取得相同完整音频，目录无可见半文件 |
| 首页/归档 WordCard、闪卡正反面及查词弹层点英文例句 | 听到该条完整英文，中文译文不读；按钮不翻面、不打开查词 |
| 例句播放结束再点一次；比较请求计数、目录和数据库，检查 no-store 与 Blob URL 清理 | 每次完成后的重播有新合成请求，例句音频不持久保存 |
| 快速从 A 例句切到 B 单词、换闪卡、关闭弹层、离开页面 | 仅最终有效目标可播放；已启动单词生成可完成保存，旧例句不留存 |
| 无凭证、供应商限流/超时/中断、目标目录不可写 | 明确错误，不回退豆包、不发布坏缓存、不无休止自动重试 |
| 后台 `/admin` 保存音色并刷新，切换查词开关；已有单词、新词、例句分别试听 | stella/tide 配置生效；已有缓存继续使用；查词开关保持原语义 |

真实供应商试听和生产音频卷保护验收须在实现阶段使用测试环境/安全样本完成，本次已运行自动测试，未发起真实付费合成或访问生产；UI 和真实发音仍待人工验收。

### 已定位的测试基础

- 后端：Go 1.24，使用 testing/httptest 和手写 repository fake；`CONTRIBUTING.md:106` 明确 `go test ./...` 为本地单测入口。已存在 doubao_test.go、settings_test.go、routes_test.go 等。
- CodeGraph affected 对 settings.go、pronunciation.go、doubao.go 均返回 `ℹ No test files affected by the changed files.`；但真实存在旧协议解析测试和配置/路由测试，不能因此判定免测。
- 关键历史兼容、鉴权及跨入口播放需要针对性测试。删除旧豆包解析测试，补新的 HTTP 音频与缓存测试，不保留旧协议适配器用于测试。
- 前端 package.json 没有测试脚本；未找到排除 node_modules 后的前端测试用例。使用 Node 内置 node:test 建立播放状态最小基线，无需额外安装框架；页面操作和英语音质仍需人工验收。
- 规划阶段只定位入口；实现阶段的实际执行结果已单独记录在 03-review.md。

## 改动清单与依赖顺序

### 一、DeepSeek 适配器和配置

- [x] `backend/internal/app/deepseek_tts.go`（新增）——实现单一 DeepSeek HTTP TTS 客户端：URL、Bearer、WAV、受限完整响应、错误分类和 context；不引入 WebSocket或聊天模型参数——deepseek_tts_test.go 验证真实协议形状。
- [x] `backend/internal/app/deepseek_tts_test.go`（新增）——httptest 覆盖成功、鉴权请求、URL /v1 拼接、429、超时、非音频、断流/超限——运行 TestDeepSeekTTS 系列。
- [x] `backend/internal/app/doubao.go`、`backend/internal/app/doubao_test.go`（删除）——删除旧供应商调用和 JSON/base64 协议解析，不触碰 audio 目录——代码引用检查及 Go 全量测试验证无残留依赖。
- [x] `backend/internal/app/settings.go`——ttsConfig 改为 DeepSeek APIKey/BaseURL/VoiceID；复用已有 DeepSeek 凭证；删除豆包播种/迁移/GET/PUT 字段；新增 deepseek_tts_voice_id / tts_voice_id 与 stella/tide 校验——配置测试、后台保存刷新验证。
- [x] `backend/internal/app/settings_test.go`——补配置 fake/音色、掩码不覆盖真实 Key、旧豆包 Key 不被使用、查词关闭仍可取语音配置——运行 TestSettings/TestTTS 系列。

### 二、单词持久缓存、例句 API 与任务管理

依赖第一组的配置和适配器。

- [x] `backend/internal/app/pronunciation.go`——保留旧 MP3 读取，新增 hashed WAV；统一读取/缺失判断/同词生成/原子保存；保存失败返回明确错误；添加只合成不保存的 handlePronounceExample；writeAudio 按实际格式返回 MIME——缓存、并发、例句和磁盘错误测试。
- [x] `backend/internal/app/app.go`——新增短生命周期同词任务表、TTS 并发限额、停止标记和任务等待状态；保持与查词锁/费用账本分开——同词合并、并发和停止测试。
- [x] `backend/internal/app/lifecycle.go`——语音任务纳入取消、拒绝新任务及有限等待；检查 WaitGroup Add/Wait 顺序——关闭竞态及 go test -race 验证。
- [x] `backend/internal/app/words.go`——保留成功查词后的异步预生成入口，更新供应商说明，确保调用统一缓存流程而不是直接覆写——与 GET 竞争及旧音频保护场景。
- [x] `backend/internal/app/routes.go`——保留 GET 单词 URL，新增鉴权 POST `/api/pronounce/example`；仅两个语音路由使用匹配的超时预算——完整路由栈鉴权和超时测试。
- [x] `backend/internal/app/pronunciation_test.go`（新增）——临时目录/上游替身验证旧字节及 mtime 不变、MP3/WAV 优先级、落盘和重读、同词合成一次、等待方取消、关闭、空/超限例句、no-store、无例句文件/数据库保存——TestPronounce/TestTTS 和 race。
- [x] `backend/internal/app/routes_test.go`——补例句 POST 未登录拒绝及带鉴权调用；保留原 GET 路由——完整 HTTP Handler 测试。

### 三、共用播放状态与所有例句入口

依赖第二组接口；播放核心可先用测试替身开发。

- [x] `frontend/src/utils/audioPlayback.js`（新增）——唯一请求/Audio、目标与来源标识、乱序丢弃、同目标加载防重、取消及 URL 释放；无持久缓存——Node 状态测试。
- [x] `frontend/src/utils/audioPlayback.test.js`（新增）——A 慢 B 快、同目标防重、结束后例句重播重新请求、owner 清理、取消和 URL 释放——node --test。
- [x] `frontend/src/composables/usePronunciation.js`——保留 speakWord 兼容调用，添加 speakExample/stopSpeech/播放状态，沿用登录 Bearer 和提示方式，接入播放核心——两类响应、鉴权和自动播放策略验收。
- [x] `frontend/src/components/word/WordCard.vue`——每条非空英文例句旁加朗读按钮及请求状态，click.stop；组件卸载时按来源清理——首页/归档、分词查词、高亮和多词性例句验证。
- [x] `frontend/src/views/FlashcardView.vue`——正面 frontExample、背面每条英文例句按钮；切词/离页清理旧来源；保留自动读词、V 键、喇叭和结果点读——正反面、快捷键、快速切卡验证。
- [x] `frontend/src/components/word/WordLookupTooltip.vue`——增加每条例句按钮，换词/关闭按来源清理——弹层点读、不误关弹层、不误停新来源验证。
- [x] `frontend/src/components/admin/AdminSettings.vue`——移除豆包三字段，用 stella/tide 音色选择；说明共用 DeepSeek 配置；保留现有保存反馈及渐变/查词配置——后台保存、刷新和非法值验证。

### 四、说明与最终行为验证

- [x] `.env.example`——增加可选 `DEEPSEEK_TTS_VOICE_ID` 说明，保留既有未提交修改，不改密钥或聊天模型配置——确认新增配置与首次播种语义一致。
- [x] `docker-compose.yml`——仅传入可选音色默认值；原音频挂载及卷名保持原状——合并 compose 配置静态查看，禁用任何删卷操作。
- [x] `README.md`、`CONTRIBUTING.md`、`DEPLOYMENT.md`——同步 DeepSeek 语音、已有 MP3/新 WAV、单词持久缓存与例句不保存、共享凭证/音色、TTS 不纳入既有日额度及旧卷保护——逐项对照最终实现和手动验收结果。
- [x] `docs/ai/DeepSeek语音迁移/03-review.md`（后续实现阶段才创建）——由主智能体记录实际改动、运行命令、未验证内容和人工验收清单；按浩原要求减少独立复核，不自动启动独立 review——如实交付，不把编译成功当音质验收。

## 关键依赖与本次不做

关键路径：配置/适配器 → 单词缓存与临时例句接口 → 播放核心与展示入口 → 联调/音频保护/文档。没有必要改释义表、词库同步、聊天 API、训练服务或用户费用账本。

规划之后已获 sw-impl 授权完成实现。本次范围不包含旧音频全量重生成、删除或转码、供应商切换、豆包回退、例句批量预生成/下载收藏、ASR/评分、语速/口音控件、多实例锁、提交推送/部署或飞书归档。

## 实施风险与前置检查

| 事项 | 当前结论 | 后续检查 |
|---|---|---|
| 凭证/网关 | 共用现有 DeepSeek Key/Base URL；生产值未知 | 实现联调检查 `/v1/tools/tts` 支持、余额与实际权限 |
| WAV 完整性 | 缓冲后校验，写完才原子发布 | 用真实短词及例句核对官方 WAV 头/流式结束特征，不根据猜测拒绝正常响应 |
| 历史音频 | 全程只读；有效旧 MP3 优先 | 测试样本哈希/mtime，以及现有外部卷挂载；不执行 docker compose down -v |
| 存储异常 | 尽早验证可写，保存失败报错 | 模拟磁盘/权限错误；承认极端失败后手动重试可能再次收费 |
| 语音日额度 | 沿用既有用户确认“不含语音” | 文档说明真实供应商支出，不擅自扩建账本 |
| 前端资源 | 共用单播放器+来源识别+URL 释放 | 模拟乱序和真实点击，重点检查弹层/闪卡交叉操作 |
| 未提交内容 | 既有 .env.example 和 frontend/pnpm-lock.yaml 保留 | 实现前再次记录 git 状态，避免把其他任务变更混入 |

实现已获授权并完成；听感、实际延迟及浏览器自动播放由浩原手动验收。

## 实施补充记录

- 按原计划保留单次音频缓冲，再原子发布 WAV；使用 App 内独立语音 WaitGroup、并发信号量与短生命周期任务表，不新增依赖。
- 新旧音频目录按 App 实例保存，测试可独立指定临时目录；初始化仍使用 AUDIO_DIR。
- 闪卡监听当前 ID 与词键，纠错改变词键时停止旧音频并读新词；翻面与普通释义补全不触发重复自动朗读。
- 原计划 UI 操作/音质验证由用户手动验收；本次完成对应文件实现、编译及播放核心自动测试，未将 UI 验收勾成通过。
- npm run lint 曾自动简化无关 highlight.js 的三个正则，本任务已恢复该文件原内容；所有语音改动文件额外通过不带 fix 的 ESLint 验证。

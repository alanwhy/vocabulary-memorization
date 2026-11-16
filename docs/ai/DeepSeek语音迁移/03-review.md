# DeepSeek 语音迁移实现与验证记录

生产发布已经完成，结果与恢复过程见 [04-release.md](04-release.md)；本文件保留实现阶段的验证记录，真实听感与 UI 仍待用户验收。

任务状态以 [01-spec.md](01-spec.md) 为准；实现计划和契约见 [02-plan.md](02-plan.md)、[02b-design.md](02b-design.md)。

阶段记录：GPT-6（具体变体未暴露）；2026-11-16 08:18:26–08:30:11，Asia/Shanghai；规范 `@sw/dev-skills@2.0.0`。

## 交付结论

已完成代码和自动验证，待浩原手动验收真实发音与页面操作。按用户要求未启动独立上下文复核，本文件为主智能体的实际结果记录。impl 阶段交付时未提交、推送、部署或归档，未访问生产配置/数据库/音频卷，未发起真实付费 TTS。

归档决定：2026-11-16 浩原回复“暂不归档，先手动验收”。本任务不执行飞书归档。

- 删除 doubao.go / doubao_test.go 与旧协议解析；后台仅显示 DeepSeek 音色选择，复用已有 DeepSeek Key/Base URL，不迁移旧供应商 Key。
- GET 单词接口先只读原位旧 MP3，再读 hashed WAV；未命中才合成，完整读完/验证并成功保存后返回。后台预生成与点击共用同词任务，完成任务移出内存。
- POST `/api/pronounce/example` 接受英文原文，返回 WAV/no-store；不读写单词缓存、不持久保存例句音频。文本上限 1,000 个计费字符，请求体 8 KiB。
- 单词卡、闪卡正反面、查词弹层增加例句按钮，保持中文译文、高亮与点击查词。共用播放器处理请求乱序、加载防重、来源关闭、取消及 Blob URL 释放。
- 现有闪卡自动读词、V 键、结果点读保留；纠错变更词键时取消旧音频并朗读新词。
- TTS 与既有用户日额度账本保持分开，遵循此前“语音不计入”的用户确认。配置、开发和部署说明已同步。

## 基线与实际文件范围

review_base：`0ba6f6ffd2c7d566198e3aa8741970e591df2172`。

起始已有 `.env.example` 末尾换行修改、未跟踪 frontend/pnpm-lock.yaml、未跟踪本任务规划文档。换行及锁文件保持原状；.env.example 仅额外添加音色默认值。未改动其他任务代码或生产外部卷配置。

| 范围 | 实际文件 |
|---|---|
| 新适配器与测试 | backend/internal/app/deepseek_tts.go、deepseek_tts_test.go；删除 doubao.go、doubao_test.go |
| 音频、任务和路由 | backend/internal/app/pronunciation.go、pronunciation_test.go、app.go、lifecycle.go、routes.go、routes_test.go、words.go |
| 配置 | backend/internal/app/settings.go、settings_test.go；frontend/src/components/admin/AdminSettings.vue；.env.example、docker-compose.yml |
| 播放与例句入口 | frontend/src/utils/audioPlayback.js、audioPlayback.test.js；frontend/src/composables/usePronunciation.js；WordCard.vue、WordLookupTooltip.vue、FlashcardView.vue |
| 文档 | README.md、CONTRIBUTING.md、DEPLOYMENT.md、docs/ai/DeepSeek语音迁移/01-spec.md、02-plan.md、02b-design.md、03-review.md |

## 实际验证

命令均在对应 backend/frontend 目录执行；所有供应商响应使用本地 httptest，音频写入临时测试目录。

| 检查 | 实际结果与边界 |
|---|---|
| RED：`go test ./internal/app -run TestPronounceRejectsInvalidWord -count=1`（实现前） | 如预期失败：../outside、apple!、123 返回 503 而非 400；实现后相应测试通过 |
| 针对性 `go test ./internal/app -run 'TestDeepSeekTTS\|TestPronounce\|TestTTS\|TestSettings\|TestHandlerRegistersExistingAPIRoutes' -count=1 -timeout=30s -v` | 通过：DeepSeek 请求字段/鉴权、URL 拼接、失败映射、WAV 断流/无效/超限、旧文件保护、新缓存复用、并发、取消、设置枚举/Key 掩码、接口鉴权 |
| `go test -race ./internal/app -run 'TestPronounce\|TestTTS' -count=1 -timeout=60s`（最终） | 通过；包含完整路由+真实鉴权中间件调用、同词后台/点击竞争、等待取消后保存、例句取消及保存发布失败测试 |
| `go vet ./...`（最终） | 通过 |
| `go test ./... -count=1 -timeout=90s`（最终） | app、billing、model、storage、training 全部通过；cmd 无用例 |
| `node --test src/utils/audioPlayback.test.js` | 7/7 通过：A/B 乱序、请求中防重、例句重播新请求、归属清理、取消、自动/手动播放错误差异、迟到 Blob 与错误释放 |
| `npm run lint` | 通过；曾自动修改无关 highlight.js，本任务已恢复原内容 |
| 对 7 个语音改动 JS/Vue 文件执行 `eslint`（不带 fix） | 通过，确认本任务文件无须依赖无关格式改动 |
| `npm run build`（最终） | 通过；仍有既有大 chunk 提示，不作为实际发音或浏览器验收依据 |
| `docker compose -f docker-compose.yml -f docker-compose.production.yml config --no-interpolate` | 静态配置可解析，仍引用原外部 audio_data 和 `/app/audio`；未运行容器、构建镜像或部署 |
| `git diff --check`、文件范围核对 | 通过；旧供应商运行时实现已移除，相关名称只在兼容说明或“不使用旧配置”的测试断言中出现 |

验证期间修正两处测试夹具问题：取消用的 HTTP 测试服务需先消费请求体才能可靠观察断连；完整路由测试需使用生产 New 构造器初始化语音生命周期，不能使用只装配鉴权依赖的旧 newTestApp。最终上述测试及竞态检查均通过，不以中途失败作为成功结果。

## 自动验证证明的行为

- 已有 MP3 在无当前 Key 时照常返回；连续点读和预生成不请求上游，旧字节与 mtime 不变；存在新 WAV 时仍优先旧 MP3。
- 新词仅生成一次并保存，另一个 App 实例在无凭证情况下直接复用该文件。
- 二十个点击请求与预生成竞争，仅一次上游请求；没有残留临时文件或永久任务表项。
- 等待方取消后，同一单词生成继续完成并保存；例句取消则终止上游请求。
- 合成返回半文件或写入/发布失败时明确返回失败，正式缓存和临时文件不会被当成成功结果。
- 同一例句两次播放产生两次上游请求，返回 no-store；未创建音频目录，例句实现无数据库依赖。
- 新例句路由未登录拒绝，完整鉴权路由能够返回 WAV；非法词键/空或超限例句不会请求供应商。

## 待浩原手动验收

1. 后台已有 DeepSeek 凭证可用时，分别试听旧词、新词，确认英语单词发音、长句自然度和首次生成等待时间；若使用自定义网关，确认支持 `/v1/tools/tts`。
2. 首页与归档页单词卡、闪卡正面首条例句、闪卡背面各词性例句、查词弹层逐一点击，确认全文英文与当前可见原文一致，中文译文不朗读。
3. 验证闪卡自动读词、V 键、正反面喇叭、结果汇总点读；例句按钮不翻面、不打开查词弹层，高亮保持原样。
4. 快速切换多个词/例句、关闭弹层、切卡或离开页面，确认最后有效目标独占播放，旧声音停止；自动播放被浏览器拦截时手动喇叭可用。
5. 后台保存 stella/tide 并刷新，确认新生成内容使用选定音色，已有单词继续复用；查看生产音频卷时确认旧文件原位保留。

实际生产音频内容与浏览器 UI 尚未验证；测试样本不能替代真实英语试听。单进程去重不保证跨多实例恰好一次；未知网络失败后的人工重试可能已经产生上游费用。当前状态为 awaiting_user。

## 后续发布授权

2026-11-16 浩原显式调用 quick-release 并回答“部署到生产环境”“无任务号”，授权版本更新、提交、推送及生产部署；归档仍按此前决定不执行。版本 1.26.0 → 2.0.0：移除旧豆包配置字段构成配置契约变更，按本次指定技能升级 major。实际部署目标为 origin/main 对应源码，经 SSH 7339 rsync 至生产 `/root/vocabulary-memorization` 后远端构建；仅清理本次明确删除的两个豆包代码文件，不删除音频数据卷。发布最终结果以本次聊天的实际执行回执为准，真实听感与 UI 仍待浩原验收。

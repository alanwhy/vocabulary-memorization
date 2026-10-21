<!-- 阶段记录：模型 GPT-6 / 开始 2026-10-21 07:46 / 结束 2026-10-21 07:54 / 规范版本 @sw/dev-skills@1.1.2 -->
# 详细设计

## 接口契约
POST /api/words/42/correct，鉴权同现有个人接口，60 秒服务端上限（模型 20 秒）。
请求：`{"reason":"过去式被误列入形近词，请核对"}`。
200 返回完整 Word，含新 senses；400 参数错误（原因去空白后 1–500 字），404 不存在/非本人，409 查询中，422 拼写错误，503 模型无有效结果，500 保存失败。
Sense 追加 `"derivatives":["worked（过去式/过去分词：工作过）"]`；缺失/null 兼容为未补齐，[] 为确认没有。时间沿用 Word 原格式。

## 后端详细设计
不改表结构。缓存 SaveSenses 条件必须包含 derivatives 缺失/null，纠错使用 ReplaceSenses 强制更新，解决原缓存按 phonetic 判断而无法覆盖的限制。后台 TranslationSenses 读取原始内容供失败恢复。WordStore 新增 TryMarkTranslating(ctx,id,now)(bool,error)，UPDATE WHERE translating=0 原子抢占。预约成功后再读取失败恢复快照，避免读到其他纠错之前的旧内容；重读失败 ClearTranslating 仅清状态，不覆盖内容。后台旧词补全保留原释义，成功整套覆盖；失败恢复旧释义。纠错预约后按词键锁串行绕过缓存重查，在 defer 恢复失败内容并释放 translating。缓存保存失败必须返回明确错误。输入原因作为待核对反馈，JSON 编码引用，不能取代词典输出约束。真实新查询必须含全部字段，每条词性的音标、例句及译文非空；词根词缀可为空字符串、关系词可为 []，仅局部结果拒绝覆盖。历史纯解析仍兼容。

### 单元测试
model：旧字段、[] 已完成、合并保留派生词；app：模型请求包含原因及全字段、模型结果派生/形近去交叉、完整 HTTP 纠错权限/空原因/重复/失败/成功及状态不变、旧闪卡缓存复用/补查去重；storage 由原子 SQL 实现预约。

## 前端详细设计
新增 WordCorrectionButton（word prop，corrected 事件），内部管理原因弹窗、提交/错误状态，成功传出服务端新 Word。HomeView/ArchiveView 同时调用闪卡队列 updateContent 同步已加载内容，保留队列进度。全局新内容先保存、个人内容后保存；若个人保存失败，恢复旧个人内容，已正确生成的全局缓存保留，接口报告保存失败。WordCard 发 corrected 事件经 WordList 交父列表局部合并；FlashcardView 就地更新卡片。对话框提交期间禁止关闭，同步禁用页面评分等状态相关操作。闪卡轮询待查卡片仅合并 senses/translating，更新不触发自动朗读；页面卸载清理定时器。队列用 contentVersions 记录本地更新版本，轮询返回只在版本未变化时合并，避免旧响应覆盖纠错新内容。

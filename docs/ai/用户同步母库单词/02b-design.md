# 详细设计（用户纠正版）

2026-11-12；Codex（GPT-6，具体子型号未知）；规范 @sw/dev-skills@1.3.3。

## 接口契约

- POST /api/me/sync-words：无业务参数，Bearer 用户鉴权，202 新任务/200 复用运行任务。客户端提供的 user_id 一律不参与目标选择。
- GET /api/me/sync-words：200 本人最近任务，未有任务返回 {"status":"idle"}。
- 运行任务样例：{"task_id":"随机ID","status":"running","added_count":0,"word_count":0}。
- 完成样例：{"task_id":"随机ID","status":"completed","added_count":12,"word_count":50}；失败带 message。
- 401 会话无效或禁用账号（沿用现有鉴权行为），503 服务正在关闭。后台事务错误以 failed 状态返回。
- 旧 POST /api/admin/users/{id}/sync-words 显式 404，防止 SPA fallback 返回伪成功。
- 请求默认 10 秒超时，仅启动/查询任务；后台任务 5 分钟超时，脱离单次 HTTP 请求取消。无分页、无时间字段、无数据库迁移。

## 后端设计

App 保存按用户 ID 索引的最近任务，mutex 保护任务快照，专用 WaitGroup 参与关闭等待。启动时以会话用户取任务，已有 running 直接返回原任务；关闭时先禁止启动，再取消 bgCtx，等待 goroutine。事务复用 SyncDictionaryToUser，过滤通过 JSON_TABLE 提取义项的 translation/pos，至少一条非 error、非空翻译才合格，非数组和 JSON null 作为空数组处理。无模型请求。

服务器重启后状态为空，页面告知重新同步；重复执行只补缺词，无次数累加。已提交数据不回滚。

## 前端设计

ChangePasswordDialog 的 action-row 放入独立 SyncDictionaryButton；组件向父层发送 busy 以禁用“重置次数”，完成发送 synced，ProfileView 重新加载统计。组件初始化查询本人状态，运行中每 2 秒查询，离开时清理定时器。确认、请求和查询等待展示加载状态，网络错误保留任务等待，不把查询失败视为任务失败；HTTP 响应不确定时核对任务 ID，旧完成任务不能误报本次成功。JS 对象表达任务状态，无 TS 改造。

## 测试覆盖

后台接口覆盖普通/管理员用户本人目标、202 异步返回、请求取消不影响后台、重复请求复用、跨用户隔离、失败/取消、旧管理员入口 404。MySQL 隔离用例补充释义过滤、历史记录保留、幂等和错误回滚；无 DSN 时明确跳过，不访问生产数据。

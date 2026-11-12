# 详细设计

阶段记录：Codex（GPT-6，具体子型号未提供）；2026-11-12 10:00（Asia/Shanghai）；规范 @sw/dev-skills@1.3.3。

## 接口契约

`POST /api/admin/users/{id}/sync-words`，无请求体，Bearer 管理员鉴权。

成功：HTTP 200，`{"added_count":12,"word_count":50}`。added_count 仅统计实际新增；word_count 包含已归档单词。空母库/全重复仍返回 200、added_count=0。

错误：400 非法正整数 ID，401 未登录，403 非管理员，404 用户不存在，500 同步失败。复用 60 秒批量请求超时，不分页、不限制母库行数；数据库执行失败回滚。该接口无时间字段。前端和后端同仓修改，必需返回两个数字字段。

## 后端详细设计

`SyncDictionaryToUser(ctx context.Context, userID int, now time.Time) (added int64, total int, err error)`：事务内 `SELECT id FROM users WHERE id=? FOR UPDATE`，缺失返回 sql.ErrNoRows；从 word_dictionary 向 words 插入不存在的 `(user_id,word_key)`，复制 display_word/senses，review_count=1，首次加入及最近复习时间=now，其他字段采用表默认值。唯一键冲突仅 `id=words.id`，不修改用户字段。按 word_key 排序减少并发锁顺序差异。统计目标用户单词数并提交；错误回滚。

| 被测入口 | 覆盖 |
|---|---|
| 新同步路由 | 管理员成功、未登录/普通用户拒绝、非法 ID、404、500；目标 ID 传递和计数返回 |
| WordRepository.SyncDictionaryToUser | 专用 MySQL 测试库：新词、重复及归档词、其他用户隔离、全重复、空母库、并发重复、失败回滚及不存在用户 |

不新增表结构，不修改母库数据，不调用外部模型。

## 前端详细设计

AdminUsers 增加按用户 ID 存储的 Set 状态。确认弹窗说明为哪个用户补齐、缺词次数为 1、已有记录保留；同步/确认阶段即锁定状态，finally 解除。同步成功使用响应 word_count 更新该行，显示 added_count；失败显示错误。同一行删除在同步中禁用。使用现有 link-btn 样式和 Vue JavaScript 数字字段，不新增全局状态或 TypeScript 定义。

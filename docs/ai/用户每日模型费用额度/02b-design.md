# 详细设计

## 接口契约

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | /api/admin/users/{id} | 返回 user（原 UserWithStats）及 budget |
| PUT | /api/admin/users/{id} | 保存 username/is_admin/disabled/daily_limit_cny，返回新详情 |
| GET | /api/me/ai-budget | 当前用户当天额度 |

编辑请求示例：`{"username":"learner","is_admin":false,"disabled":false,"daily_limit_cny":"5.00"}`；null 表示不限，"0.00" 暂停付费，额度范围 0–1,000,000 元且最多两位小数。所有编辑字段必填。budget 为 `{date,daily_limit_cny,spent_cny,reserved_cny,remaining_cny,pending_requests}`，金额为十进制字符串，剩余额度扣除预留、不小于零，不限额时 daily_limit/remaining 为 null。

404 用户不存在、409 重名或最后管理员保护、400 字段无效或操作自己、403 无管理权限。限额失败为 429 `ai_budget_exceeded`；业务失败仍保留已产生费用；异步查词以用户词条提示返回，不继续普通错误退避。

## 数据与后端

- users.daily_ai_limit_nano BIGINT NULL：新普通用户 5×10^9，新管理员 NULL；历史首次迁移补默认，不在每次启动重置。
- ai_daily_usage：主键(user_id,usage_date)，spent_nano/reserved_nano BIGINT 默认0，持久化历史。
- ai_requests：request_id 主键，user_id/date/purpose/provider/model/parent_id/reserved_nano/actual_nano/state/details/created_at。保存费率及 usage 元信息，避免存正文或 Key；日索引支持待核算查询。用户删除后保留账本用于已发请求结算，不添加级联删除。
- Reserve：锁 users → 日使用行 → 可选同用户/同日父请求；新预留检查总使用金额，子请求从工作流剩余预留转移，不重复占用；每一步重新检查禁用、当前额度。
- Settle：锁日行 → 请求行；reserved/uncertain 才可结算，释放预留并增加费用，重复调用无副作用。未知请求保留预留；未发出可明确释放。
- 工作流：先预留两次有界 chat + 一次 Search + 最多五次 Fetch；每个 HTTP 请求独立账目；退出释放尚未发出的剩余，午夜后新请求在新日重新申请。
- 用户编辑：事务锁管理员集合与目标用户，防止取消最后可用管理员；禁止禁用/降级自己。金额更新和用户字段一起提交。
- 用户归属：普通查词从词条的 user_id 取得，纠错/母库从当前登录用户取得，练习从用户所属 Word 取得；客户端 context 显式携带受信任 owner。

## 测试

计费：金额解析、缓存分类、未知 usage/模型、业务错误费用、工具结果、请求取消、父子预留、日期。存储：并发争额、调额后重新检查、重复结算和工作流释放。接口：无权限、无效金额、自己保护、重名及成功返回。真实 MySQL 事务验证使用已有仓库测试方式；缺依赖记录待验收。

## 前端

AdminUsers 使用 Vue JavaScript 局部状态。表格仅用户名/角色/操作。详情弹窗实时读接口，展示账号状态、创建/登录时间、单词数、额度、已用、预留、剩余及待核算。编辑弹窗展示用户名/角色/状态、每日金额/不限额、重置密码；保存后刷新列表和已打开详情。自己的禁用/降级控件禁用。创建用户保留，默认额度由服务器按角色确定。弹窗 loading/error 防重复操作；详情显示费用为估算以及语音不计入。

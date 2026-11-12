# 实现计划

执行授权引用 01；GPT-6；2026-11-12；frontend@1.25.1。feature：用户日金额额度及用户管理详情/编辑。

## 验证前置

| 路径/命令 | 预期 |
|---|---|
| 模拟 HTTP 供应商 → 预留 → usage 返回/业务失败 → 结算 | 缓存、推理不重复计算；未知 usage 保留占用；工具无结果不收费 |
| 并发预留/工作流拆分/重复结算/上海换日 | 不超发、不重复入账、重启后账本持久化 |
| 管理员 GET/PUT 用户详情；普通用户直接请求 | 管理员可编辑，普通用户 403；不能禁用或降级自己 |
| 用户管理 → 详情/编辑 → 保存额度/角色/状态/重置密码 | 表格仅三列三项操作；详情完整；保存即时生效 |
| go test ./...、go vet ./...、前端构建 | 基础检查通过，付费 live 测试按其环境开关跳过 |

后端已有 24 个测试文件，使用 go test；前端无测试文件，不新建页面测试。CodeGraph affected 未列出受影响测试，结合已读调用方补计费/权限测试。金额与并发属于业务规则，补必要用例；UI 最终手动验收。已通过 Go 全部非 live 用例、go vet、计费/应用/存储包 race 检查、用户管理 ESLint 和前端构建；真实 MySQL 专项测试因未配置独立测试库而跳过，不能视为事务验收通过。

## 改动清单

- [x] backend/internal/billing/{billing.go,http.go,billing_test.go}：精确金额、费率、受控 HTTP、工作流预留、异常结算；模拟供应商验证。
- [x] backend/internal/storage/{billing.go,billing_test.go,migrations.go,users.go}、backend/schema.sql：MySQL 日账本/请求账本、用户额度、原子编辑；存储规则与迁移验证。
- [x] backend/internal/app/{app.go,admin_users.go,routes.go}、backend/cmd/vocab-server/main.go：装配与管理接口、用户详情/编辑；权限测试。
- [x] backend/internal/app/{deepseek.go,translate.go,words.go,correction.go,dictionary.go,training_worker.go,settings.go}：每个实际请求带用户身份，限额错误不重试，保留已有内容；调用路径检查。
- [x] backend/internal/training/{provider.go,search.go}：统一计费客户端及输出上限，工具计费；供应商测试。
- [x] frontend/src/components/admin/AdminUsers.vue：三列表格、详情、编辑、重置密码及原创建功能；构建及用户验收。
- [x] 03-review.md：记录实际检查、限制及用户验收；不独立复核。

## 关键文件与边界

受控 HTTP 服务位于 billing，避免在两个模型客户端重复计费；新装 schema 与幂等迁移同步。现有用户 API 及会话立即失效语义保留。详情/编辑新增字段由新接口提供，不依赖过期列表快照。

费率采用官方 2026-11-12 快照；工作日高峰按高峰、其他时段按空闲，法定节假日未接入日历时高峰窗口保守估算（页面说明）。自定义供应商/未知模型有限额时阻断，不限额时统计待核算。模型输出 4096 tokens、非思考模式，输入按 UTF-8 字节上界含消息框架预留并限制长度；响应超出上界转待核算并停止本轮。

不部署、不提交、不变更语音费用与其他页面；飞书归档独立授权。

实现落点调整：复用原 User/Word 模型，以独立详情响应和账本查询提供额度及计费主体，不改 models.go；新增 app/admin_users_test.go 验证真实鉴权路由。无计划项跳过，数据库及 UI 验收留待用户。

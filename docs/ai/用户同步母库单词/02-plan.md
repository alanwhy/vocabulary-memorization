# 纠正实现计划

沿用 01-spec.md 的自动执行模式；2026-11-12，Codex（GPT-6，具体子型号未知）。

## 验证前置

普通用户进入个人中心 → 修改密码旁同步 → 等待任务 → 检查有释义缺词新增 1 次，原词不变，统计刷新；同步中刷新页面继续等待，重复点击复用任务。用户管理无同步入口，旧接口 404，查询只返回本人任务。网络查询失败继续等待；数据库错误可重试。

后端使用 go test ./... 及新异步用例的竞态检查；前端对变更组件运行 ESLint 和构建。真实数据库过滤及 UI 操作交用户手动验收；隔离 MySQL 用例无 DSN 明确跳过。

## 改动清单

- [x] backend/internal/app/user_words.go、app.go、lifecycle.go、routes.go：用户异步任务和关闭管理、替换旧入口。
- [x] backend/internal/storage/word_sync.go、word_sync_test.go：仅有释义过滤及对应数据用例。
- [x] frontend/src/components/admin/AdminUsers.vue：移除上一版同步入口及状态。
- [x] frontend/src/components/profile/SyncDictionaryButton.vue、ChangePasswordDialog.vue、frontend/src/views/ProfileView.vue：个人按钮、等待/恢复及统计刷新。
- [x] backend/internal/app/user_words_test.go：异步、幂等、权限及状态测试。
- [x] README.md、docs/ai/用户同步母库单词/03-review.md：更正使用说明、记录实际验证。

## 关键调用与范围

ProfileView → ChangePasswordDialog → SyncDictionaryButton → 用户鉴权路由 → App 后台同步 → WordRepository 事务。保留其他开发中的同义替换代码，不顺手调整；本轮不提交或再次发布，原 1.24.0 发布授权已执行完毕。

发布补充：用户本轮再次调用 quick-release，授权将上述纠正提交、推送并部署生产；沿用无任务号。版本为 1.24.1，页面操作仍交用户手动验收。

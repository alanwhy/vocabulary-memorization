执行模式：沿用本任务已确认的自动执行至编码；Lightweight 合并记录。2026-11-12；Codex（GPT-6，具体子型号未知）；frontend@1.24.1。

# 头部版本号自动同步

目标：标题旁版本号随发布版本自动更新。

事实：HomeView.vue 将显示值写死为 v1.18.1；发布更新的是 frontend/package.json 与 package-lock.json，二者没有绑定，所以线上真实版本已为 1.24.1，显示仍旧。已优先用 CodeGraph 定位，再精确检索常量。

实现：HomeView 从 package.json 导入 version，模板显示 v{{ appVersion }}。只修改此页面；不修改接口、状态、依赖或生产版本，不发布服务器。

边界：构建时取版本，后续升版后需重新构建；页面无新增状态或错误分支。已有导航、统计和录词逻辑保持，其他工作区改动保留。

验证：HomeView ESLint、前端构建与本次 diff 格式检查通过；构建后的 HomeView 文件包含 1.24.1。无需新增 UI 测试或独立复核，实际显示由用户在发布后手动验收。本修复尚未提交或发布。

影响面原始记录：

```text

Impact of changing "HomeView" — 1 affected symbols:

frontend/src/views/HomeView.vue
  component   HomeView:1
```

归档：沿用本任务暂不归档飞书。

发布准备：本轮核对到仓库和生产已到 1.25.0（4a557b4），因此本修复升补丁版本为 1.25.1。按已确认的生产环境、无任务号执行；标题为 `fix: 头部版本号自动跟随发布版本`。发布源码按原 package-lock 安装，HomeView ESLint 和构建通过，生成首页资源已包含 1.25.1。提交、推送、远端构建和最终检查结果以本次发布会话为准。

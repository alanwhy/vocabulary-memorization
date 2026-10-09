<!-- Lightweight 任务记录：模型 GPT-5 / 开始 2026-10-09 / 规范版本 @sw/dev-skills@1.1.2 -->

# 背单词列表回到顶部

## 目标与范围

在“背单词”页面的列表右下方提供回到顶部入口。页面向下滚动超过 300px 时显示，点击后平滑回到页面顶部。

本次只修改 `frontend/src/views/HomeView.vue`；不改共享的 `WordList`，避免影响归档页的无限滚动列表。

## 必要事实

- 背单词页和词表使用整页滚动，没有独立的列表滚动容器。
- 项目已经全局启用 Element Plus，内置 `el-backtop` 正好监听页面滚动；不传 `target` 才能正确作用于页面。
- `HomeView` 的 CodeGraph 影响面只包含其自身组件，属于单页 UI 增强；前端没有测试文件或可运行的测试脚本。

## 实现与验收

- [x] 在 `WordList` 后添加仅属于背单词页的 `el-backtop`。
- [x] 设为右侧 24px、底部 32px，并在页面滚动超过 300px 后显示。
- [x] 执行 `cd frontend && npm run build`，确认 Vue 模板可编译。

## 验证结果

2026-10-09 已执行 `cd frontend && npm run build`，Vite 构建成功。构建仅提示既有 bundle 体积告警，未产生编译错误。

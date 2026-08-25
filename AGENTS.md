<!-- 由 team-dev-skills 的入口 skill 自动创建，内容固定，不需要按项目填写。改动请改本仓库的 templates/AGENTS.md 源文件。 -->

# 本项目遵循 team-dev-skills 四阶段开发规范

四阶段：`/sw-spec`（需求与设计）→ `/sw-plan`（实现计划）→ `/sw-impl`（编码）→ `/sw-review`（自审与验收，可选）。
产物落在 `docs/ai/<中文描述>/`（目录名由 `/sw-spec` 自动生成），看目录里已有哪些文件就知道进行到哪一步。

## 用 Claude Code 或 Codex

四个入口已经是 skill，装过 `team-dev-skills/install.sh` 后可直接触发：
Claude Code 里敲 `/sw-spec` 等；Codex 里同名 skill 会按描述自动匹配，或显式 `@sw-spec` 调用。
不需要读本文件其余部分。

## 用 Cursor / 其他没有 skill 机制的工具

没有 `/sw-spec` 这类命令时，靠人工判断当前该走哪一步：

1. 看 `docs/ai/<中文描述>/` 下已有哪些产物：没有任何文件 → 从需求分析开始；有 `01-*.md` 无 `02-plan.md` → 该做计划；以此类推。
2. 打开 `~/.claude/team-dev-skills/dist/prompts/sw-<stage>.md`（`spec`/`plan`/`impl`/`review` 四选一，每份都是自包含文件），整份内容当作系统指令喂给当前对话。

## 不分工具都要遵守的硬规则

- **定制逻辑不动公共层**：给单个客户加需求时，不要改公共组件/公共接口/公共工具。今天满足这个客户，下个月别的客户升级时容易炸，而 diff 看起来小而合理、人工 review 容易漏。

## 完整规范

`~/.claude/team-dev-skills/dist/PLAYBOOK.md`（由源文件拼接生成，人读的和 AI 用的是同一批源，不会漂移）。

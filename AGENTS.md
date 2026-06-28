<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

## 项目前端说明

- 前端源码放在 `apps/web/src/`。
- 分支、PR、提交和合并策略以 `CONTRIBUTING.md` 为准；当前前端工作默认通过个人 fork 向主仓库 `develop` 发起 PR。
- 面向团队成员的协作流程见 `docs/frontend-workflow.md`。
- 面向 agent 的前端规范见 `.trellis/spec/frontend/index.md`。
- 涉及前端开发、分支、PR、Lint 或 CI 时，优先加载项目级 `frontend-workflow` skill。

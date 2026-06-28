# Document frontend technology stack guidelines

## Goal

Persist the confirmed frontend technology stack and implementation conventions for this repository so future frontend work has a clear, project-local reference.

## What I already know

- The user provided a public ChatGPT shared conversation titled "前端技术栈梳理".
- The shared conversation recommends a frontend stack for an AI-enabled power-industry management platform covering knowledge management, intelligent Q&A, report generation, RBAC, and dashboards.
- The repository already contains `.trellis/spec/frontend/` guideline placeholders.
- `AGENTS.md` currently contains a Trellis-managed block and should not receive large frontend-specific guidance.

## Requirements

- Update `.trellis/spec/frontend/` with concrete frontend conventions derived from the shared conversation.
- Keep the guidance concise, actionable, and project-scoped.
- Cover:
  - recommended stack
  - module/page responsibilities
  - directory structure
  - component conventions
  - state management
  - type safety
  - quality expectations
- Add only a short pointer outside `.trellis/spec/frontend/` if needed.
- Do not create a Codex skill for this task; the content is project specification, not a reusable workflow/tool integration.

## Acceptance Criteria

- [ ] `.trellis/spec/frontend/index.md` no longer reads as a placeholder and names the chosen frontend stack.
- [ ] Frontend directory, component, state, quality, and type-safety guidance is populated.
- [ ] `AGENTS.md` is left unchanged unless a minimal index pointer is clearly useful.
- [ ] No application code or dependencies are changed.
- [ ] Git diff is reviewed before final response.

## Definition of Done

- Documentation updates are complete.
- Markdown is readable and consistent.
- `git diff --check` passes.
- Final response reports changed files and skipped checks, if any.

## Out of Scope

- Building the frontend application.
- Adding package dependencies.
- Creating a project skill.
- Creating reusable component templates or scaffolding scripts.

## Technical Notes

- Source conversation: `https://chatgpt.com/s/t_6a3fa2dd4c748191aa29a022ba960efb`
- Local fetched copy during analysis: `/tmp/chatgpt-share.html`
- Preferred persistence target: `.trellis/spec/frontend/`

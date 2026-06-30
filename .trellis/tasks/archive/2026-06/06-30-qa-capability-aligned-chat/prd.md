# F-016 QA chat capability aligned citations and tools

## Goal

Implement issue #282 so the QA Chat page displays only backend-backed, safe QA SSE content, citation state, tool summaries, and RAG degradation/error state. The frontend must stay based on the latest `upstream/develop` and must not invent citation details, RAG results, private reasoning, internal URLs, object keys, raw MCP payloads, full prompts, or provider raw errors.

## Requirements

- Base the work on `upstream/develop@109dc74` in branch `Frontend/feat/qa-capability-aligned-chat`.
- Use public Gateway `/api/v1/**` contracts only; do not call QA, Knowledge, AI Gateway, MCP, or file service internal addresses from browser code.
- QA SSE handling must use the documented events: `message.created`, `agent.iteration.started`, `reasoning.step`, `tool.started`, `tool.completed`, `tool.failed`, `answer.delta`, `citation.delta`, `answer.completed`, `error`, and optional `heartbeat`.
- Tool UI must use only QA-provided sanitized fields such as tool name, status, sanitized summary fields, latency, and sanitized error code/message. It must not render full raw event payloads.
- Reasoning UI must render only `reasoning.step` safe summaries supplied by QA.
- Citation UI must render citation markers/cards from QA citation snapshots, and must explicitly state that source detail is unavailable or pending when detail is not present. Since #93 is still open, do not pretend citation detail lookup is fully ready.
- Knowledge retrieval/backend dependency failures must show a RAG degradation/error state with `requestId`, or clearly state when no `requestId` is available.
- Non-2xx SSE setup failures must parse the Gateway error envelope instead of showing raw response text.

## Acceptance Criteria

- [ ] No private chain-of-thought, full prompt, internal URL, object key, raw tool arguments/results, or provider raw error is displayed.
- [ ] Citation UI displays snapshots only and communicates unavailable/pending source detail.
- [ ] QA stream and dependency failures surface `requestId` when Gateway provides it.
- [ ] Tool summary UI is derived from explicit sanitized fields, not raw payload dumping.
- [ ] Browser code continues to call only public Gateway paths.

## Definition of Done

- `bun run --cwd apps/web check` passes.
- `bun run --cwd apps/web build` passes.
- `git diff --check` passes.
- Focused unit tests cover the new QA capability/error helpers.

## Out of Scope

- Implementing QA citation backend, Knowledge retrieval, MCP runtime, or cross-service smoke.
- Updating project docs unless implementation discovers a durable rule that belongs in `.trellis/spec/`.
- Claiming #84/#93 capabilities are fully ready when current docs/issues still show gaps.

## Technical Notes

- Issue #282 is the task source: <https://github.com/Sakayori-Iroha-168/Software_Teamwork/issues/282>.
- #93 remains open for QA citation snapshot/detail/batch lookup; frontend must not fabricate source detail.
- #84 is closed, but current Knowledge implementation docs and capability matrix still identify `knowledge-queries`/retrieval closure and smoke as partial or risky; frontend must show graceful degradation.
- `docs/collaboration/frontend-readiness-task-plan.md` is referenced by #282 but does not exist in latest `develop`; use current docs instead and do not edit docs.
- Primary docs consulted: `docs/architecture/frontend-backend-contract.md`, `docs/architecture/current-capability-matrix.md`, `docs/services/gateway/api/openapi.yaml`, `docs/services/gateway/docs/active-api-owner-map.md`, `docs/services/qa/docs/implementation.md`, `docs/services/knowledge/docs/implementation.md`.

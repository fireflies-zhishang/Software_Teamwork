# fix PR 258 frontend test isolation

## Goal

Address the PR review findings by separating frontend test TypeScript context from production app compilation and by resetting API client module-level state between tests.

## Requirements

- Keep production app TypeScript config free of Vitest and Testing Library globals.
- Ensure production app typecheck/build does not include `*.test.*`, `*.spec.*`, or test setup files.
- Add a dedicated test TypeScript config for unit/component test files and test setup.
- Keep test type checking in the frontend quality gate without polluting production tsconfig.
- Reset API client singleton/module state between tests, including token, providers, unauthorized handler, and mock routes.

## Acceptance Criteria

- [x] `apps/web/tsconfig.app.json` includes production source only and no test global types.
- [x] Test files continue to typecheck via a dedicated test config.
- [x] `apps/web/src/test/setup.ts` resets API client state around tests.
- [x] Existing unit, e2e, check, build, and whitespace checks pass.
- [x] No public documentation is changed for this PR review fix.

## Definition of Done

- Relevant code/config changes are committed.
- Bun frontend checks pass from the repository root.
- PR #258 is updated from the fork branch.
- Trellis task is archived after completion.

## Technical Approach

- Create `apps/web/tsconfig.test.json` extending the app compiler options while overriding test-only `types`, `include`, `exclude`, and build info location.
- Remove `vitest/globals` and `@testing-library/jest-dom` from `apps/web/tsconfig.app.json`, and exclude tests plus `src/test/**`.
- Add `typecheck:test` to the frontend package scripts and run it as part of `check`.
- Export a test reset helper from `apps/web/src/api/client.ts`; call it from `apps/web/src/test/setup.ts` in both `beforeEach` and `afterEach`.

## Decision (ADR-lite)

Context: Review identified that the initial test baseline allowed test globals in production TypeScript and allowed API client singleton state to leak across tests.

Decision: Use a separate test tsconfig plus an explicit API client reset helper instead of keeping test types in production config or relying on each test file to remember cleanup.

Consequences: Production and test type contexts are separated. Test files still receive explicit type checking through `bun run --cwd apps/web typecheck:test`. The API client exposes a test-only reset helper, which is a small public export but keeps cleanup centralized and reliable.

## Out of Scope

- Reworking the API client architecture beyond test state reset.
- Changing public collaboration docs.
- Changing Playwright or Vitest dependency versions.

## Technical Notes

- Review feedback points to `apps/web/tsconfig.app.json`, `apps/web/src/test/setup.ts`, and `apps/web/src/api/client.test.ts`.
- Current frontend commands use Bun from the repository root: `bun run --cwd apps/web <script>`.

## Progress Log

- Implemented production/test TypeScript config split with `apps/web/tsconfig.test.json`.
- Added API client test reset helper and global setup cleanup.
- Captured the test type boundary convention in `.trellis/spec/frontend/quality-guidelines.md`.
- Verification passed: `bun run --cwd apps/web check`, `bun run --cwd apps/web test:unit`, `bun run --cwd apps/web build`, `bun run --cwd apps/web test:e2e`, and `git diff --check`.

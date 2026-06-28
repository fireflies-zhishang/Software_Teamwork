# Hook Guidelines

> Custom hooks, data fetching hooks, and streaming hooks.

## Overview

Hooks should isolate reusable stateful logic without hiding important product flow. Prefer small hooks with clear ownership over large "god hooks".

## Naming

- Use `useXxx` for all hooks.
- Use `use<Domain><Resource>Query` for TanStack Query readers.
- Use `use<Domain><Action>Mutation` for TanStack Query writers.
- Use `use<Feature>Stream` for SSE/fetch-stream logic.
- Use `use<Feature>Filters` only when filter state is reused or complex.

## Data Fetching

- Use TanStack Query for server reads and writes.
- Centralize query keys and query option factories inside the relevant feature folder.
- Mutations must invalidate or update affected queries explicitly.
- Use polling for long-running document/report tasks when SSE is not available.
- Keep generated API calls in `api/generated/`; wrap them in feature-level query functions when UI needs domain-specific behavior.

Example organization:

```txt
features/knowledge/
  knowledge.queries.ts
  knowledge.mutations.ts
  hooks/
    use-document-processing-status.ts
```

## SSE and Streaming

- Put shared stream handling in `lib/sse.ts`.
- Use explicit event types: `start`, `delta`, `citation`, `reasoning`, `progress`, `done`, `error`.
- Streaming hooks must support cancellation through `AbortController`.
- Streaming hooks must expose enough state for UI: `status`, `content`, `progress`, `error`, and domain-specific payloads such as `citations` or generated sections.
- Never assume a stream completes successfully. Handle partial content and user cancellation.

## Form Hooks

- Use React Hook Form directly in forms unless the form has reusable domain behavior.
- Keep Zod schemas next to the feature form or in `features/<domain>/schemas/`.
- Do not duplicate schema defaults between hooks and components; export default values from the schema module when needed.

## Common Mistakes

- Creating hooks that only wrap one `useState` call and are never reused.
- Hiding query invalidation inside unrelated UI components.
- Storing API responses in local component state instead of using TanStack Query.
- Forgetting cleanup for streams, polling, timers, or event listeners.

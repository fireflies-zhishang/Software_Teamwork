# State Management

> Rules for local, URL, server, persisted, and global state.

## State Categories

| Category | Owner | Examples |
| --- | --- | --- |
| Server state | TanStack Query | Knowledge bases, documents, report records, user profile, settings. |
| Local UI state | React state | Dialog open state, active tab, local form toggles. |
| URL state | TanStack Router search params | Filters, pagination, selected tab when shareable/bookmarkable. |
| Global client state | Zustand | Sidebar collapsed state, theme, chat draft/session cache, auth shell hints. |
| Form state | React Hook Form | Login, knowledge base settings, model config, report parameters. |
| Persisted browser state | Zustand persist or IndexedDB/localStorage | Chat session list, local drafts, UI preferences. |

## Server State

- Server-owned data belongs in TanStack Query.
- Prefer query invalidation after mutations over manually synchronizing many local stores.
- Use query keys that include all relevant filters and route params.
- Use optimistic updates only for low-risk UI interactions with clear rollback behavior.

## URL State

Put state in the URL when:

- The state affects list contents.
- The state should survive refresh.
- The state should be shareable or bookmarkable.

Examples: table page, page size, search keyword, document status filter, knowledge base type, retrieval test parameters when useful.

## Zustand

Use Zustand for small global client state only:

- Current UI theme and sidebar state.
- Chat local session list and unsent drafts.
- Lightweight auth shell state derived from `/api/me`.
- Cross-route UI preferences.

Do not put paginated lists, documents, report records, model settings, or permission matrices in Zustand if they come from the backend.

## Chat State

- Store server-backed chat history through backend APIs when available.
- Use local persistence for client-only drafts and temporary session recovery.
- Message state should distinguish `pending`, `streaming`, `done`, and `error`.
- Persist enough metadata to restore sessions after refresh, but avoid caching sensitive documents or credentials in localStorage.

Recommended shape:

```ts
type ChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  status: 'pending' | 'streaming' | 'done' | 'error'
  citations?: Citation[]
  reasoningSteps?: ReasoningStep[]
  createdAt: string
}
```

## Long-Running Tasks

- Document parsing/vectorization and report generation must expose explicit frontend states.
- Prefer backend task status endpoints or SSE events over inferred frontend timers.
- UI should show progress, retry/failure actions, and final output links where available.

## Common Mistakes

- Mirroring TanStack Query data into Zustand.
- Keeping filters only in component state when they should survive refresh.
- Persisting secrets, API keys, or sensitive source content in browser storage.
- Treating streaming content as a final answer before the `done` event.

# Type Safety

> TypeScript, OpenAPI, Zod, and runtime validation rules.

## Core Rules

- Use TypeScript for all frontend code.
- Prefer generated API types from OpenAPI when backend contracts exist.
- Validate user input and untrusted runtime data with Zod.
- Keep route params and search params typed through TanStack Router.
- Avoid `any`; use `unknown` plus validation when the shape is not known.

## API Types

- Store generated clients/types under `apps/web/src/api/generated/`.
- Do not manually edit generated files.
- Wrap generated calls in feature-level functions when UI needs domain naming, query keys, or response normalization.
- Keep frontend DTO mapping explicit when backend response shape is not UI-ready.

## Zod Schemas

Use Zod for:

- Login and registration forms.
- Knowledge base create/edit forms.
- Retrieval parameter forms: Top K, similarity threshold, rerank threshold, selected knowledge bases.
- Model configuration forms: API URL, model name, timeout, credentials placeholders.
- Report generation parameters.
- Report outline and section save payloads when edited client-side.

Infer form value types from schemas:

```ts
const retrievalSettingsSchema = z.object({
  topK: z.number().int().min(1).max(100),
  similarityThreshold: z.number().min(0).max(1),
  rerankThreshold: z.number().min(0).max(1).optional(),
})

type RetrievalSettingsForm = z.infer<typeof retrievalSettingsSchema>
```

## Domain Types

Define domain types for important client-side structures:

```ts
type Citation = {
  documentId: string
  documentName: string
  chunkId: string
  content: string
  score: number
  sectionPath?: string
}

type ReportOutlineNode = {
  id: string
  title: string
  level: number
  kind: 'text' | 'table' | 'image'
  children?: ReportOutlineNode[]
}
```

Prefer generated backend types for persisted entities and explicit frontend types for UI-only state.

## Discriminated Unions

Use discriminated unions for status-heavy UI:

- Document processing status.
- Upload item status.
- Chat message status.
- Report section generation status.
- Long task status.

Example:

```ts
type UploadItemState =
  | { status: 'queued'; file: File }
  | { status: 'uploading'; file: File; progress: number }
  | { status: 'done'; documentId: string }
  | { status: 'failed'; file: File; message: string }
```

## Forbidden Patterns

- `any` for API responses, form values, route params, or event payloads.
- Blind `as` assertions to force types through compile errors.
- Duplicating backend DTO types by hand when generated types exist.
- Allowing untyped search params into query keys.
- Treating streamed JSON chunks as trusted without parsing and validation.

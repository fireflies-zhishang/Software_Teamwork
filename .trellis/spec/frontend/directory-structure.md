# Directory Structure

> Frontend module organization and file layout.

## Standard Layout

This repository is expected to contain both frontend and backend code, so do not put frontend source directly under repository-root `src/`. Use `apps/web/` as the frontend application root. Backend code should live in its own sibling app/service directory selected by the backend stack.

Use this structure when creating the frontend application:

```txt
apps/
  web/
    package.json
    vite.config.ts
    tsconfig.json
    src/
      app/
        router.tsx
        providers.tsx
        query-client.ts
      layouts/
        app-layout.tsx
        auth-layout.tsx
      pages/
        login/
        knowledge/
          libraries/
          documents/
          chunks/
          search/
        qa/
          chat/
          retrieval-test/
          settings/
        reports/
          generate/
          records/
          templates/
        system/
          users/
          roles/
          settings/
      components/
        ui/
        common/
        data-table/
        file-upload/
        markdown/
        rich-editor/
        charts/
      features/
        auth/
        knowledge/
        qa/
        reports/
        system/
      api/
        client.ts
        generated/
      stores/
        auth-store.ts
        ui-store.ts
        chat-store.ts
      lib/
        utils.ts
        permissions.ts
        sse.ts
        download.ts
```

## Responsibilities

- `apps/web/`: frontend application root.
- `apps/web/src/app/`: application wiring only: router, providers, query client, global error boundaries.
- `apps/web/src/layouts/`: route shells such as authenticated AppShell and unauthenticated AuthLayout.
- `apps/web/src/pages/`: route-level composition. Pages should orchestrate feature components, not contain reusable business logic.
- `apps/web/src/features/`: domain-specific components, hooks, schemas, and helpers grouped by product module.
- `apps/web/src/components/ui/`: shadcn/ui generated primitives. Keep them generic.
- `apps/web/src/components/common/`: cross-domain reusable UI with minimal product assumptions.
- `apps/web/src/components/data-table/`: table shell, filters, pagination, column helpers, row actions.
- `apps/web/src/components/file-upload/`: reusable upload/dropzone components and progress UI.
- `apps/web/src/components/markdown/`: markdown renderer, citation renderer, code block renderer.
- `apps/web/src/components/rich-editor/`: TipTap editor shell, toolbar, report/table extensions.
- `apps/web/src/components/charts/`: shared chart wrappers and dashboard metric components.
- `apps/web/src/api/generated/`: generated OpenAPI client/types. Do not manually edit generated files.
- `apps/web/src/stores/`: small Zustand stores for durable UI/client state only.
- `apps/web/src/lib/`: framework-agnostic utilities.

## Module Boundaries

- Knowledge management code lives under `apps/web/src/features/knowledge/` and `apps/web/src/pages/knowledge/`.
- Intelligent Q&A code lives under `apps/web/src/features/qa/` and `apps/web/src/pages/qa/`.
- Report generation code lives under `apps/web/src/features/reports/` and `apps/web/src/pages/reports/`.
- Auth and RBAC helpers live under `apps/web/src/features/auth/`, `apps/web/src/stores/auth-store.ts`, and `apps/web/src/lib/permissions.ts`.
- Shared UI must not import feature modules.

## Naming Conventions

- Use kebab-case for files and folders: `report-outline-editor.tsx`.
- Use PascalCase for React components: `ReportOutlineEditor`.
- Use `*.schema.ts` for Zod schemas.
- Use `*.types.ts` only when types are shared by multiple files and are not generated.
- Use `*.queries.ts` for TanStack Query option factories and query keys.
- Use `*.mutations.ts` for mutation helpers when a module has multiple write operations.
- Use `use-*.ts` for hooks.

## Page Organization

Each non-trivial page directory may contain:

```txt
page.tsx
components/
hooks/
schemas/
utils.ts
```

Move reusable domain logic from page-local folders into `features/<domain>/` once it is used by more than one page.

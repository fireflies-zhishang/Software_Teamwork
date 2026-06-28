# Frontend Development Guidelines

> Project-specific frontend standards for the power-industry AI management platform.

## Overview

The frontend is expected to support an AI-enabled management system with four product areas:

- Authentication, RBAC, layout, navigation, and theme support.
- Knowledge management: knowledge bases, document upload, document processing status, chunks, retrieval configuration, and search.
- Intelligent Q&A: multi-session chat, SSE streaming answers, RAG retrieval, citations, reasoning steps, and retrieval testing.
- Report generation: report type selection, outline editing, section streaming, rich text editing, records, templates, and DOCX download.

This repository is expected to contain both frontend and backend code. The frontend application should live under `apps/web/`; references to `src/` in frontend specs mean `apps/web/src/`, not repository-root `src/`.

## Recommended Stack

Use this stack unless a task has an explicit reason to diverge:

| Layer | Choice | Purpose |
| --- | --- | --- |
| Build | Vite | React SPA development and production builds. |
| Language | TypeScript | API, form, permission, report, and UI state contracts. |
| UI framework | React | Component model and app runtime. |
| Routing | TanStack Router | Type-safe nested routes and route guards. |
| Server state | TanStack Query | API caching, pagination, invalidation, polling, and mutations. |
| Local global state | Zustand | Auth shell state, theme, sidebar, and chat draft/session state. |
| Styling | Tailwind CSS | Layout, responsive design, and theme tokens. |
| Components | shadcn/ui + Radix UI | Accessible dialogs, popovers, forms, tabs, dropdowns, toast, command palette, and primitives. |
| Tables | TanStack Table | Knowledge bases, documents, report records, users, and system lists. |
| Forms | React Hook Form + Zod | Auth, model settings, retrieval settings, report parameters, and template settings. |
| Upload | react-dropzone | Drag-and-drop and click-to-select uploads. |
| Markdown | react-markdown + remark-gfm | AI answers, code, tables, and citation-aware rendering. |
| Rich text | TipTap / ProseMirror | Report section and template editing. |
| Drag sorting | dnd-kit | Report outline and block ordering. |
| Charts | Recharts | 30-day trends and dashboard metrics. |
| Icons | lucide-react | Menus, actions, and status indicators. |
| API types | OpenAPI + openapi-typescript or Orval | Generated API client/types from backend contracts. |
| Tests | Vitest + React Testing Library + Playwright | Unit, component, and critical workflow E2E tests. |
| Formatting | ESLint + Prettier + Husky + lint-staged | Team consistency and pre-commit checks. |

## Guideline Index

Read these files before frontend implementation:

| Guide | Description | Status |
| --- | --- | --- |
| [Directory Structure](./directory-structure.md) | Module organization and file layout. | Active |
| [Component Guidelines](./component-guidelines.md) | Component patterns, props, composition, and module UI choices. | Active |
| [Hook Guidelines](./hook-guidelines.md) | Custom hooks, data fetching hooks, and SSE hooks. | Active |
| [State Management](./state-management.md) | Local, URL, server, persisted, and global state rules. | Active |
| [Quality Guidelines](./quality-guidelines.md) | Required checks, forbidden patterns, and review expectations. | Active |
| [Type Safety](./type-safety.md) | TypeScript, OpenAPI, Zod, and runtime validation rules. | Active |

## Merge Adaptation Notes

The frontend spec was merged from two sources:

- `upstream/develop` already had a frontend spec structure and repository collaboration rules. Those files established that frontend guidance belongs under `.trellis/spec/frontend/` and that repository-level branch/PR policy belongs in `CONTRIBUTING.md`.
- L1ngg's previous frontend work filled the placeholders with concrete implementation decisions for `apps/web`: Vite, React, TypeScript, TanStack Router/Query, Zustand, Tailwind, shadcn/Radix, Bun commands, and the AI management product modules.

The merged result keeps the `develop` ownership model: `CONTRIBUTING.md` remains the source of truth for branch, PR, commit, and merge policy. The frontend spec only defines how to implement and verify frontend code under `apps/web/`.

When the two versions overlapped, prefer this rule:

1. Keep `develop` repository policy and Trellis structure.
2. Keep L1ngg's concrete frontend stack, directory layout, and quality commands when they refine placeholder or generic guidance.
3. Do not put branch targets such as `frontend-dev` in frontend specs unless `CONTRIBUTING.md` first adopts them.
4. Preserve backend-facing constraints from `develop` by describing frontend integration through backend contracts instead of coupling browser code to backend internals.

## Product Module Priorities

Build the first usable frontend around these flows:

1. Login/session restore, AppShell layout, RBAC route/menu filtering.
2. Knowledge base list/detail and document upload with processing status.
3. Chat with SSE streaming, multi-session history, citations, and reasoning steps.
4. Report generation wizard: type selection, outline generation/editing, streaming section generation, rich text editing, and DOCX download.
5. Retrieval test page, report records, template management, dashboards, and theme polish.

## Backend Integration Contract

Frontend and backend should converge early on:

- Authentication: prefer HttpOnly cookie session for the management UI unless backend chooses JWT explicitly.
- API documentation: backend publishes OpenAPI; frontend uses generated types/clients.
- Pagination shape: `page`, `pageSize`, `total`, `items`.
- Error shape: `code`, `message`, optional `details`.
- Uploads: `multipart/form-data`; backend returns document/template IDs and processing status.
- Long tasks: document parsing, vectorization, and report generation expose task status consistently.
- SSE events: use explicit event types such as `start`, `delta`, `citation`, `reasoning`, `progress`, `done`, and `error`.
- Permissions: frontend hides unauthorized UI, backend enforces authorization.
- Downloads: original documents and generated DOCX files are downloaded through backend endpoints.

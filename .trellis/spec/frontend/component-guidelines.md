# Component Guidelines

> Component patterns, props, composition, and UI choices.

## Core Principles

- Build dense, utilitarian management interfaces. Avoid marketing-style hero sections and decorative card-heavy layouts for operational tools.
- Prefer predictable tables, forms, sidebars, drawers, dialogs, tabs, and detail pages over custom interaction patterns.
- Use shadcn/ui and Radix primitives first. Create custom primitives only when the existing primitives cannot model the interaction.
- Use lucide-react icons in icon buttons and menu items when available.
- Keep page components as composition shells. Put reusable behavior in domain feature components or hooks.

## Component Layers

- Route page: loads route params/search state, composes features, handles route-level loading/error states.
- Feature component: implements product workflows such as upload panels, chat windows, report outline editors.
- Shared component: generic table, upload, markdown, editor, chart, empty state, and confirm dialog primitives.
- UI primitive: shadcn/ui components with minimal local styling.

## Props

- Define props with `type`, not `interface`, unless extension is required.
- Keep props explicit and narrow. Avoid passing entire API response objects to deeply nested children when only a few fields are needed.
- Prefer controlled components for forms, filters, dialogs, and editors.
- Use discriminated unions for components with modes, such as upload item status or chat message status.

Example:

```ts
type ProcessingStatus = 'uploaded' | 'parsing' | 'chunking' | 'vectorizing' | 'ready' | 'failed'

type DocumentStatusBadgeProps = {
  status: ProcessingStatus
  errorMessage?: string
}
```

## Product UI Patterns

### Authentication and Layout

- Use `AppShell` for authenticated pages: `Sidebar + Header + Breadcrumb + Content`.
- Use route guards for authenticated and role-restricted routes.
- Filter menus and actions by permission, but rely on backend authorization as the source of truth.

### Knowledge Management

- Use `TanStack Table` for knowledge bases, documents, chunks, report records, users, and settings lists.
- Use badges for processing statuses: uploaded, parsing, chunking, vectorizing, ready, failed.
- Use `Dialog` for small create/edit forms, `Drawer` or detail pages for large content such as chunk details.
- Use `ConfirmDialog` for destructive actions such as delete and batch delete.
- Use `react-dropzone`-based upload components with progress, retry, and error states.

### Intelligent Q&A

- Render assistant output with `react-markdown + remark-gfm`.
- Model messages with explicit statuses: pending, streaming, done, error.
- Use a dedicated citation component for citation markers.
- Use `HoverCard` or `Popover` for short citation previews and `Drawer` for longer source previews.
- Use `Collapsible` or a timeline component for reasoning steps; collapse automatically after streaming finishes.

### Report Generation

- Use a wizard flow: report type -> parameters -> outline -> section generation -> edit -> export.
- Use tree/list components plus `dnd-kit` for outline ordering.
- Use TipTap for report section editing and template visual editing.
- Keep DOCX generation in the backend or a document service. The frontend sends structured report data and handles download.

## Styling

- Use Tailwind utility classes and shadcn theme variables.
- Do not hard-code one-off colors when a theme token exists.
- Keep cards for repeated list/detail items, modal bodies, and framed tools. Do not wrap full page sections in nested cards.
- Use stable dimensions for tables, icon buttons, toolbar controls, chat messages, and report outline rows to avoid layout shifts.

## Accessibility

- Preserve Radix accessibility behavior.
- Icon-only buttons must have accessible labels and tooltips when the icon is not obvious.
- Forms must show validation messages near the relevant field.
- Upload and streaming states must expose loading/progress text in addition to visual indicators.

## Forbidden Patterns

- Duplicating shadcn/ui primitives into feature folders.
- Storing server data inside Zustand when TanStack Query should own it.
- Building ad hoc tables instead of using the shared table pattern for management lists.
- Rendering untrusted HTML directly from AI responses or uploaded content.
- Hiding permission failures only in frontend code without backend enforcement.

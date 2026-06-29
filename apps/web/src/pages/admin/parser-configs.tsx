import { Edit, FileCode, Loader2, Plus, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  useCreateParserConfig,
  useDeleteParserConfig,
  useParserConfigs,
  useUpdateParserConfig,
} from '@/features/admin-config'
import type { ParserConfig } from '@/lib/types'

// ── Constants ──

const BACKEND_OPTIONS = [
  { value: 'builtin', label: '内置' },
  { value: 'tika', label: 'Apache Tika' },
  { value: 'unstructured', label: 'Unstructured' },
  { value: 'local_ocr', label: '本地 OCR' },
  { value: 'remote_compatible', label: '远程兼容' },
] as const

const BACKEND_LABELS: Record<string, string> = {
  builtin: '内置',
  tika: 'Apache Tika',
  unstructured: 'Unstructured',
  local_ocr: '本地 OCR',
  remote_compatible: '远程兼容',
}

// ── Types ──

interface FormData {
  name: string
  backend: string
  concurrency: number
  fileTypes: string
  chunkSize: number
  chunkOverlap: number
  separators: string
  endpointUrl: string
}

type NotificationState = {
  type: 'success' | 'error'
  text: string
}

// ── Defaults ──

const EMPTY_FORM: FormData = {
  name: '',
  backend: 'builtin',
  concurrency: 4,
  fileTypes: '',
  chunkSize: 512,
  chunkOverlap: 64,
  separators: '',
  endpointUrl: '',
}

// ── Helpers ──

function formToCreateRequest(form: FormData) {
  const params: Record<string, unknown> = {
    name: form.name,
    backend: form.backend,
    concurrency: form.concurrency,
    enabled: true,
    isDefault: false,
  }

  if (form.backend === 'remote_compatible' && form.endpointUrl.trim()) {
    params.endpointUrl = form.endpointUrl.trim()
  }

  if (form.fileTypes.trim()) {
    params.supportedContentTypes = form.fileTypes
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
  }

  params.defaultParameters = {
    chunk_size: form.chunkSize,
    chunk_overlap: form.chunkOverlap,
    separators: form.separators.trim()
      ? form.separators
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
      : [],
  }
  return params
}

function formToUpdateRequest(form: FormData) {
  const params: Record<string, unknown> = {
    name: form.name,
    backend: form.backend,
    concurrency: form.concurrency,
    endpointUrl: form.backend === 'remote_compatible' ? form.endpointUrl.trim() || null : null,
    supportedContentTypes: form.fileTypes.trim()
      ? form.fileTypes
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
      : [],
    defaultParameters: {
      chunk_size: form.chunkSize,
      chunk_overlap: form.chunkOverlap,
      separators: form.separators.trim()
        ? form.separators
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean)
        : [],
    },
  }
  return params
}

// ── Skeleton ──

function ParserConfigsSkeleton() {
  return (
    <div className="animate-pulse space-y-4">
      <div className="flex items-center justify-between">
        <div className="h-7 w-32 rounded bg-muted" />
        <div className="h-8 w-36 rounded bg-muted" />
      </div>
      <div className="rounded-lg border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <div className="grid grid-cols-5 gap-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="h-4 rounded bg-muted" />
            ))}
          </div>
        </div>
        <div className="divide-y divide-border">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="grid grid-cols-5 gap-4 px-4 py-3">
              {Array.from({ length: 5 }).map((_, j) => (
                <div key={j} className="h-4 rounded bg-muted" />
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ── Main component ──

export function ParserConfigsPage() {
  // ── State ──
  const [createOpen, setCreateOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [editingConfig, setEditingConfig] = useState<ParserConfig | null>(null)
  const [deletingConfig, setDeletingConfig] = useState<ParserConfig | null>(null)

  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [notification, setNotification] = useState<NotificationState | null>(null)

  // ── Queries & mutations ──

  const { data, isLoading, isError, error, refetch } = useParserConfigs()

  const createMutation = useCreateParserConfig()
  const updateMutation = useUpdateParserConfig()
  const deleteMutation = useDeleteParserConfig()

  const isMutating =
    createMutation.isPending || updateMutation.isPending || deleteMutation.isPending

  // ── Notification auto-dismiss ──

  useEffect(() => {
    if (!notification) return
    const timer = setTimeout(() => setNotification(null), 4000)
    return () => clearTimeout(timer)
  }, [notification])

  // ── Handlers ──

  const updateField = useCallback(
    <K extends keyof FormData>(field: K, value: FormData[K]) => {
      setForm((prev) => ({ ...prev, [field]: value }))
    },
    [],
  )

  const openCreate = useCallback(() => {
    setForm(EMPTY_FORM)
    setCreateOpen(true)
  }, [])

  const openEdit = useCallback((config: ParserConfig) => {
    setEditingConfig(config)
    setForm({
      name: config.name,
      backend: config.backend,
      concurrency: config.concurrency,
      fileTypes: config.supportedContentTypes?.join(', ') ?? '',
      chunkSize: (config.defaultParameters?.chunk_size as number) ?? 512,
      chunkOverlap: (config.defaultParameters?.chunk_overlap as number) ?? 64,
      separators:
        config.defaultParameters?.separators != null
          ? (config.defaultParameters.separators as string[]).join(', ')
          : '',
      endpointUrl: config.endpointUrl ?? '',
    })
    setEditOpen(true)
  }, [])

  const openDelete = useCallback((config: ParserConfig) => {
    setDeletingConfig(config)
    setDeleteOpen(true)
  }, [])

  const handleCreate = useCallback(() => {
    createMutation.mutate(
      formToCreateRequest(form) as Parameters<typeof createMutation.mutate>[0],
      {
        onSuccess: () => {
          setNotification({ type: 'success', text: '解析器配置创建成功' })
          setCreateOpen(false)
        },
        onError: (err: Error) => {
          setNotification({ type: 'error', text: `创建失败: ${err.message}` })
        },
      },
    )
  }, [form, createMutation])

  const handleEdit = useCallback(() => {
    if (!editingConfig) return
    updateMutation.mutate(
      {
        id: editingConfig.id,
        ...formToUpdateRequest(form),
      } as Parameters<typeof updateMutation.mutate>[0],
      {
        onSuccess: () => {
          setNotification({ type: 'success', text: '解析器配置更新成功' })
          setEditOpen(false)
          setEditingConfig(null)
        },
        onError: (err: Error) => {
          setNotification({ type: 'error', text: `更新失败: ${err.message}` })
        },
      },
    )
  }, [editingConfig, form, updateMutation])

  const handleDelete = useCallback(() => {
    if (!deletingConfig) return
    deleteMutation.mutate(deletingConfig.id, {
      onSuccess: () => {
        setNotification({ type: 'success', text: '解析器配置已删除' })
        setDeleteOpen(false)
        setDeletingConfig(null)
      },
      onError: (err: Error) => {
        setNotification({ type: 'error', text: `删除失败: ${err.message}` })
      },
    })
  }, [deletingConfig, deleteMutation])

  // ── Derived ──

  const items = data ?? []
  const isEmpty = !isLoading && !isError && items.length === 0

  // ── Render ──

  return (
    <div>
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h3 className="text-2xl font-semibold text-foreground">解析器配置</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            管理文档解析器配置，支持新建、编辑、删除操作。
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus aria-hidden="true" className="mr-1 size-4" />
          新建配置
        </Button>
      </div>

      {/* Toast notification */}
      {notification && (
        <div
          role="alert"
          className={`toast-enter mb-4 rounded-lg border px-4 py-3 text-sm ${
            notification.type === 'success'
              ? 'border-emerald-500/50 bg-emerald-50 text-emerald-800 dark:border-emerald-400/30 dark:bg-emerald-950 dark:text-emerald-300'
              : 'border-destructive/50 bg-destructive/10 text-destructive'
          }`}
        >
          {notification.text}
        </div>
      )}

      {/* Loading state */}
      {isLoading && <ParserConfigsSkeleton />}

      {/* Error state */}
      {isError && !isLoading && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-6 text-center">
          <p className="mb-3 text-sm text-destructive">
            加载解析器配置失败: {error instanceof Error ? error.message : '未知错误'}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <Loader2 aria-hidden="true" className="mr-1.5 size-3.5" />
            重试
          </Button>
        </div>
      )}

      {/* Empty state */}
      {isEmpty && (
        <div className="rounded-lg border border-dashed border-border p-12 text-center">
          <FileCode
            aria-hidden="true"
            className="mx-auto mb-3 size-10 text-muted-foreground/40"
          />
          <p className="text-sm text-muted-foreground">
            暂无解析器配置，点击新建配置开始
          </p>
          <Button variant="outline" size="sm" className="mt-3" onClick={openCreate}>
            <Plus aria-hidden="true" className="mr-1 size-3.5" />
            新建配置
          </Button>
        </div>
      )}

      {/* Table */}
      {!isLoading && !isError && items.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/30">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
                  名称
                </th>
                <th className="hidden px-4 py-2.5 text-left font-medium text-muted-foreground sm:table-cell">
                  后端
                </th>
                <th className="hidden px-4 py-2.5 text-left font-medium text-muted-foreground md:table-cell">
                  文件类型
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
                  分块大小
                </th>
                <th className="hidden px-4 py-2.5 text-left font-medium text-muted-foreground sm:table-cell">
                  分块重叠
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
                  状态
                </th>
                <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">
                  操作
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {items.map((config) => {
                const chunkSize =
                  (config.defaultParameters?.chunk_size as number) ?? '-'
                const chunkOverlap =
                  (config.defaultParameters?.chunk_overlap as number) ?? '-'

                return (
                  <tr
                    key={config.id}
                    className="transition-colors duration-150 hover:bg-muted/30"
                  >
                    <td className="max-w-36 truncate px-4 py-2.5 font-medium text-foreground">
                      {config.name}
                    </td>
                    <td className="hidden px-4 py-2.5 sm:table-cell">
                      <Badge variant="secondary">
                        {BACKEND_LABELS[config.backend] ?? config.backend}
                      </Badge>
                    </td>
                    <td className="hidden max-w-40 truncate px-4 py-2.5 text-muted-foreground md:table-cell">
                      {config.supportedContentTypes?.join(', ') ?? '-'}
                    </td>
                    <td className="px-4 py-2.5 tabular-nums text-muted-foreground">
                      {typeof chunkSize === 'number' ? chunkSize : '-'}
                    </td>
                    <td className="hidden px-4 py-2.5 tabular-nums text-muted-foreground sm:table-cell">
                      {typeof chunkOverlap === 'number' ? chunkOverlap : '-'}
                    </td>
                    <td className="px-4 py-2.5">
                      {config.enabled ? (
                        <Badge variant="default">启用</Badge>
                      ) : (
                        <Badge variant="secondary">禁用</Badge>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => openEdit(config)}
                          aria-label={`编辑 ${config.name}`}
                        >
                          <Edit aria-hidden="true" className="size-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => openDelete(config)}
                          aria-label={`删除 ${config.name}`}
                          className="text-destructive hover:text-destructive"
                        >
                          <Trash2 aria-hidden="true" className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* ── Create Dialog ── */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建解析器配置</DialogTitle>
            <DialogDescription>配置文档解析器的基本参数。</DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            {/* Name */}
            <div>
              <label
                htmlFor="pc-create-name"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                名称 <span className="text-destructive">*</span>
              </label>
              <Input
                id="pc-create-name"
                type="text"
                placeholder="解析器配置名称"
                value={form.name}
                onChange={(e) => updateField('name', e.target.value)}
              />
            </div>

            {/* Backend */}
            <div>
              <label
                htmlFor="pc-create-backend"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                解析后端 <span className="text-destructive">*</span>
              </label>
              <select
                id="pc-create-backend"
                value={form.backend}
                onChange={(e) => updateField('backend', e.target.value)}
                className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm text-foreground transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm"
              >
                {BACKEND_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            {/* Conditional: endpointUrl (remote_compatible) */}
            {form.backend === 'remote_compatible' && (
              <div>
                <label
                  htmlFor="pc-create-endpointurl"
                  className="mb-1 block text-sm font-medium text-foreground"
                >
                  远程地址
                </label>
                <Input
                  id="pc-create-endpointurl"
                  type="url"
                  placeholder="https://parser-api.example.com/v1"
                  value={form.endpointUrl}
                  onChange={(e) => updateField('endpointUrl', e.target.value)}
                />
              </div>
            )}

            {/* Concurrency */}
            <div>
              <label
                htmlFor="pc-create-concurrency"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                并发数 <span className="text-destructive">*</span>
              </label>
              <Input
                id="pc-create-concurrency"
                type="number"
                placeholder="4"
                min={1}
                max={128}
                value={form.concurrency}
                onChange={(e) =>
                  updateField(
                    'concurrency',
                    Math.min(128, Math.max(1, Number(e.target.value) || 1)),
                  )
                }
              />
            </div>

            {/* File Types */}
            <div>
              <label
                htmlFor="pc-create-filetypes"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                文件类型
              </label>
              <Input
                id="pc-create-filetypes"
                type="text"
                placeholder="pdf, docx, txt (逗号分隔)"
                value={form.fileTypes}
                onChange={(e) => updateField('fileTypes', e.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                支持的文件 MIME 类型或扩展名，逗号分隔。
              </p>
            </div>

            {/* Chunk Size */}
            <div>
              <label
                htmlFor="pc-create-chunksize"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分块大小
              </label>
              <Input
                id="pc-create-chunksize"
                type="number"
                placeholder="512"
                min={1}
                value={form.chunkSize}
                onChange={(e) =>
                  updateField('chunkSize', Math.max(1, Number(e.target.value) || 1))
                }
              />
            </div>

            {/* Chunk Overlap */}
            <div>
              <label
                htmlFor="pc-create-chunkoverlap"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分块重叠
              </label>
              <Input
                id="pc-create-chunkoverlap"
                type="number"
                placeholder="64"
                min={0}
                value={form.chunkOverlap}
                onChange={(e) =>
                  updateField('chunkOverlap', Math.max(0, Number(e.target.value) || 0))
                }
              />
            </div>

            {/* Separators */}
            <div>
              <label
                htmlFor="pc-create-separators"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分隔符（可选）
              </label>
              <Input
                id="pc-create-separators"
                type="text"
                placeholder="\n\n, \n, 。(逗号分隔)"
                value={form.separators}
                onChange={(e) => updateField('separators', e.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                文本分段使用的分隔符，逗号分隔。留空使用默认设置。
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)} disabled={isMutating}>
              取消
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!form.name.trim() || isMutating}
            >
              {createMutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-1.5 size-3.5 animate-spin" />
              )}
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Edit Dialog ── */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑解析器配置</DialogTitle>
            <DialogDescription>
              修改 "{editingConfig?.name}" 的配置信息。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            {/* Name */}
            <div>
              <label
                htmlFor="pc-edit-name"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                名称 <span className="text-destructive">*</span>
              </label>
              <Input
                id="pc-edit-name"
                type="text"
                placeholder="解析器配置名称"
                value={form.name}
                onChange={(e) => updateField('name', e.target.value)}
              />
            </div>

            {/* Backend */}
            <div>
              <label
                htmlFor="pc-edit-backend"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                解析后端 <span className="text-destructive">*</span>
              </label>
              <select
                id="pc-edit-backend"
                value={form.backend}
                onChange={(e) => updateField('backend', e.target.value)}
                className="h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm text-foreground transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm"
              >
                {BACKEND_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            {/* Conditional: endpointUrl (remote_compatible) */}
            {form.backend === 'remote_compatible' && (
              <div>
                <label
                  htmlFor="pc-edit-endpointurl"
                  className="mb-1 block text-sm font-medium text-foreground"
                >
                  远程地址
                </label>
                <Input
                  id="pc-edit-endpointurl"
                  type="url"
                  placeholder="https://parser-api.example.com/v1"
                  value={form.endpointUrl}
                  onChange={(e) => updateField('endpointUrl', e.target.value)}
                />
              </div>
            )}

            {/* Concurrency */}
            <div>
              <label
                htmlFor="pc-edit-concurrency"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                并发数 <span className="text-destructive">*</span>
              </label>
              <Input
                id="pc-edit-concurrency"
                type="number"
                placeholder="4"
                min={1}
                max={128}
                value={form.concurrency}
                onChange={(e) =>
                  updateField(
                    'concurrency',
                    Math.min(128, Math.max(1, Number(e.target.value) || 1)),
                  )
                }
              />
            </div>

            {/* File Types */}
            <div>
              <label
                htmlFor="pc-edit-filetypes"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                文件类型
              </label>
              <Input
                id="pc-edit-filetypes"
                type="text"
                placeholder="pdf, docx, txt (逗号分隔)"
                value={form.fileTypes}
                onChange={(e) => updateField('fileTypes', e.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                支持的文件 MIME 类型或扩展名，逗号分隔。
              </p>
            </div>

            {/* Chunk Size */}
            <div>
              <label
                htmlFor="pc-edit-chunksize"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分块大小
              </label>
              <Input
                id="pc-edit-chunksize"
                type="number"
                placeholder="512"
                min={1}
                value={form.chunkSize}
                onChange={(e) =>
                  updateField('chunkSize', Math.max(1, Number(e.target.value) || 1))
                }
              />
            </div>

            {/* Chunk Overlap */}
            <div>
              <label
                htmlFor="pc-edit-chunkoverlap"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分块重叠
              </label>
              <Input
                id="pc-edit-chunkoverlap"
                type="number"
                placeholder="64"
                min={0}
                value={form.chunkOverlap}
                onChange={(e) =>
                  updateField('chunkOverlap', Math.max(0, Number(e.target.value) || 0))
                }
              />
            </div>

            {/* Separators */}
            <div>
              <label
                htmlFor="pc-edit-separators"
                className="mb-1 block text-sm font-medium text-foreground"
              >
                分隔符（可选）
              </label>
              <Input
                id="pc-edit-separators"
                type="text"
                placeholder="\n\n, \n, 。(逗号分隔)"
                value={form.separators}
                onChange={(e) => updateField('separators', e.target.value)}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                文本分段使用的分隔符，逗号分隔。留空使用默认设置。
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setEditOpen(false)
                setEditingConfig(null)
              }}
              disabled={isMutating}
            >
              取消
            </Button>
            <Button onClick={handleEdit} disabled={!form.name.trim() || isMutating}>
              {updateMutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-1.5 size-3.5 animate-spin" />
              )}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Delete Confirmation Dialog ── */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认删除</DialogTitle>
            <DialogDescription>
              确定要删除解析器配置 "{deletingConfig?.name}" 吗？此操作不可撤销。
            </DialogDescription>
          </DialogHeader>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setDeleteOpen(false)
                setDeletingConfig(null)
              }}
              disabled={isMutating}
            >
              取消
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={isMutating}>
              {deleteMutation.isPending && (
                <Loader2 aria-hidden="true" className="mr-1.5 size-3.5 animate-spin" />
              )}
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

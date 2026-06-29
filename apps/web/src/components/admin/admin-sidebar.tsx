import { Link, useRouterState } from '@tanstack/react-router'
import { BarChart3, ChevronDown, ChevronLeft, ChevronRight, Database, FileText, LayoutTemplate, MessageSquareText, Package, Settings, Wrench } from 'lucide-react'
import { useState } from 'react'

import type { AdminMenuItem } from '@/lib/types'
import { cn } from '@/lib/utils'
import { useUiStore } from '@/stores/ui-store'

const menuItems: AdminMenuItem[] = [
  {
    key: 'system',
    label: '系统管理',
    children: [
      { key: 'users', label: '用户管理', path: '/admin/users' },
      { key: 'roles', label: '角色管理', path: '/admin/roles' },
      { key: 'styles', label: '样式管理', path: '/admin/styles' },
      { key: 'report-categories', label: '报告类别', path: '/admin/report-categories' },
      { key: 'files', label: '文件管理', path: '/admin/files' },
    ],
  },
  {
    key: 'stats',
    label: 'QA 统计',
    path: '/admin/stats',
  },
  {
    key: 'reports',
    label: '报告生成',
    children: [
      { key: 'report-records', label: '报告记录', path: '/admin/reports/records' },
      { key: 'report-templates', label: '模板素材', path: '/admin/reports/templates' },
    ],
  },
  {
    key: 'templates',
    label: '模板管理',
    path: '/admin/templates',
  },
  {
    key: 'materials',
    label: '材料管理',
    path: '/admin/materials',
  },
  {
    key: 'prompts',
    label: '提示词管理',
    path: '/admin/prompts',
  },
  {
    key: 'rag',
    label: 'RAG 知识库',
    children: [
      { key: 'knowledge', label: '知识管理', path: '/admin/knowledge' },
      { key: 'knowledge-config', label: '知识配置', path: '/admin/knowledge-config' },
      { key: 'knowledge-experience', label: '知识体验', path: '/admin/knowledge-experience' },
      { key: 'qa-settings', label: 'QA / LLM 配置', path: '/admin/qa-settings' },
      { key: 'qa-retrieval-test', label: 'QA 检索测试', path: '/admin/qa-retrieval-test' },
    ],
  },
  {
    key: 'settings',
    label: '系统设置',
    path: '/admin/settings',
  },
]

const ICON_MAP: Record<string, typeof Settings> = {
  system: Settings,
  stats: BarChart3,
  reports: FileText,
  templates: LayoutTemplate,
  materials: Package,
  prompts: MessageSquareText,
  rag: Database,
  settings: Wrench,
}

export function AdminSidebar() {
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const [expanded, setExpanded] = useState<Set<string>>(new Set(['system', 'reports', 'rag']))
  const sidebarCollapsed = useUiStore((s) => s.sidebarCollapsed)
  const toggleSidebar = useUiStore((s) => s.toggleSidebar)

  const toggle = (key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const handleGroupClick = (key: string) => {
    if (sidebarCollapsed) toggleSidebar()
    toggle(key)
  }

  const isActive = (path?: string): boolean => {
    if (!path) return false
    return pathname === path || pathname.startsWith(`${path}/`)
  }

  return (
    <aside
      className={cn(
        'flex flex-shrink-0 flex-col border-r border-border bg-sidebar overflow-hidden',
        'transition-[width] duration-300',
        sidebarCollapsed ? 'w-14' : 'w-56',
      )}
    >
      {/* Header: title + toggle */}
      <div className="flex items-center border-b border-border">
        {!sidebarCollapsed && (
          <h2 className="flex-1 whitespace-nowrap px-4 py-3 text-sm font-semibold text-sidebar-foreground">
            管理面板
          </h2>
        )}
        <button
          type="button"
          onClick={toggleSidebar}
          className={cn(
            'flex shrink-0 items-center justify-center text-muted-foreground transition-all hover:bg-accent hover:text-foreground',
            sidebarCollapsed ? 'mx-auto my-3 size-7 rounded-md' : 'mr-1 size-7 rounded-md',
          )}
          aria-label={sidebarCollapsed ? '展开侧边栏' : '折叠侧边栏'}
        >
          {sidebarCollapsed ? (
            <ChevronRight className="size-4 transition-transform duration-300" />
          ) : (
            <ChevronLeft className="size-4 transition-transform duration-300" />
          )}
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex flex-1 flex-col gap-0.5 overflow-auto py-1">
        {menuItems.map((item) => {
          const hasChildren = item.children && item.children.length > 0
          const Icon = ICON_MAP[item.key]

          if (hasChildren) {
            const open = expanded.has(item.key) && !sidebarCollapsed
            return (
              <div key={item.key}>
                <button
                  type="button"
                  className={cn(
                    'flex w-full items-center text-left text-sm font-medium text-sidebar-foreground transition-colors hover:bg-primary/5 hover:text-primary',
                    sidebarCollapsed ? 'justify-center px-0 py-2' : 'gap-1.5 px-4 py-2',
                  )}
                  onClick={() => handleGroupClick(item.key)}
                  title={sidebarCollapsed ? item.label : undefined}
                >
                  {sidebarCollapsed ? (
                    Icon && <Icon className="size-5 shrink-0" />
                  ) : (
                    <>
                      {open ? (
                        <ChevronDown aria-hidden="true" size={12} className="shrink-0 text-muted-foreground" />
                      ) : (
                        <ChevronRight aria-hidden="true" size={12} className="shrink-0 text-muted-foreground" />
                      )}
                      <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
                      <span className="whitespace-nowrap">{item.label}</span>
                    </>
                  )}
                </button>
                {open && (
                  <div className="bg-sidebar-accent/40 py-0.5 transition-[max-height] duration-200">
                    {item.children!.map((child) => (
                      <Link
                        key={child.key}
                        to={child.path!}
                        className={cn(
                          'block whitespace-nowrap px-4 py-1.5 pl-10 text-sm text-muted-foreground transition-colors hover:bg-primary/5 hover:text-primary',
                          isActive(child.path) && 'text-primary bg-primary/10 font-medium',
                        )}
                      >
                        {child.label}
                      </Link>
                    ))}
                  </div>
                )}
              </div>
            )
          }

          return (
            <Link
              key={item.key}
              to={item.path!}
              className={cn(
                'flex items-center text-sm font-medium text-sidebar-foreground transition-colors hover:bg-primary/5 hover:text-primary',
                sidebarCollapsed ? 'justify-center px-0 py-2' : 'px-4 py-2',
                isActive(item.path) && 'text-primary bg-primary/10 font-medium',
              )}
              title={sidebarCollapsed ? item.label : undefined}
            >
              {sidebarCollapsed && Icon ? (
                <Icon className="size-5 shrink-0" />
              ) : (
                <span className="whitespace-nowrap">{item.label}</span>
              )}
            </Link>
          )
        })}
      </nav>
    </aside>
  )
}

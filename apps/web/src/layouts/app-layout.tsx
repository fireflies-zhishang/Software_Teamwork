import { apiClient } from '@/api/client'
import { Link, useRouter, useRouterState } from '@tanstack/react-router'
import { Loader2, LogOut, RefreshCw, ShieldAlert } from 'lucide-react'
import type { PropsWithChildren, ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import type { PermissionRequirement } from '@/lib/permissions'
import { canAccess } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth-store'

const pathLabels: Record<string, string> = {
  '/chat': '智能问答',
  '/reports': '报告生成',
  '/admin': '系统管理',
  '/forbidden': '权限不足',
}

const navItems: Array<{
  label: string
  to: '/chat' | '/reports' | '/admin'
  requirement?: PermissionRequirement
}> = [
  { label: '问答', to: '/chat', requirement: { any: ['qa:use'] } },
  {
    label: '报告',
    to: '/reports',
    requirement: { any: ['report:read', 'report:write', 'reports:write'] },
  },
  {
    label: '管理',
    to: '/admin',
    requirement: {
      any: [
        'system:admin',
        'report:read',
        'report:write',
        'reports:write',
        'knowledge:read',
        'knowledge:write',
        'document:upload',
        'admin:model-profile:write',
        'admin:parser-config:write',
      ],
    },
  },
]

function FullPageState({
  action,
  children,
  title,
}: PropsWithChildren<{ action?: ReactNode; title: string }>) {
  return (
    <div className="flex h-full items-center justify-center bg-background p-6 text-foreground">
      <section className="w-full max-w-md rounded-lg border border-border bg-card p-6 text-center shadow-sm">
        <h1 className="text-lg font-semibold">{title}</h1>
        <div className="mt-2 text-sm text-muted-foreground">{children}</div>
        {action && <div className="mt-5">{action}</div>}
      </section>
    </div>
  )
}

export function AppLayout({ children }: PropsWithChildren) {
  const router = useRouter()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const currentLabel =
    Object.entries(pathLabels).find(([key]) => pathname.startsWith(key))?.[1] ?? '首页'
  const user = useAuthStore((state) => state.user)
  const status = useAuthStore((state) => state.status)
  const error = useAuthStore((state) => state.error)
  const restoreSession = useAuthStore((state) => state.restoreSession)

  if (status === 'restoring' || status === 'idle') {
    return (
      <FullPageState title="正在恢复会话">
        <span className="inline-flex items-center gap-2">
          <Loader2 className="size-4 animate-spin" />
          正在读取当前用户信息
        </span>
      </FullPageState>
    )
  }

  if (status === 'error') {
    return (
      <FullPageState
        action={
          <Button variant="outline" onClick={() => void restoreSession()}>
            <RefreshCw className="size-4" />
            重试
          </Button>
        }
        title="会话恢复失败"
      >
        {error ?? '无法读取当前用户，请稍后重试。'}
      </FullPageState>
    )
  }

  const visibleNavItems = navItems.filter((item) => canAccess(user, item.requirement))

  const handleLogout = () => {
    const token = apiClient.getToken()
    void router.navigate({ to: '/login' })
    // Fire-and-forget: end the server session BEFORE clearing local token
    if (token && token !== 'dev-token-bypass') {
      void fetch(`${apiClient.baseUrl}/sessions/current`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => { /* best-effort */ })
    }
    useAuthStore.getState().clearSession()
  }

  return (
    <div className="flex h-full flex-col bg-background text-foreground">
      <header className="flex h-14 items-center justify-between border-b border-primary/30 bg-primary/5 px-6">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
            电
          </div>
          <span className="truncate text-sm text-primary">电力行业知识助手</span>
          <span className="truncate text-sm font-semibold">{currentLabel}</span>
        </div>

        <nav className="flex items-center gap-1 text-sm">
          {visibleNavItems.map((item) => (
            <Link
              key={item.to}
              to={item.to}
              className="rounded-md px-2 py-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              activeProps={{ className: 'text-primary bg-primary/10' }}
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="hidden max-w-40 truncate sm:inline">{user?.username ?? '未登录'}</span>
          {user && user.roles.length > 0 && (
            <span className="hidden rounded-md bg-muted px-2 py-1 sm:inline">
              {user.roles.join(', ')}
            </span>
          )}
          {!visibleNavItems.length && (
            <span className="inline-flex items-center gap-1 text-destructive">
              <ShieldAlert className="size-3.5" />
              无可用菜单
            </span>
          )}
          <Button
            aria-label="退出登录"
            size="icon-sm"
            type="button"
            variant="ghost"
            onClick={handleLogout}
          >
            <LogOut />
          </Button>
        </div>
      </header>

      <main className="flex-1 overflow-hidden">{children}</main>
    </div>
  )
}

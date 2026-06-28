import {
  BarChart3,
  BookOpen,
  FileText,
  MessageSquareText,
  Settings,
  ShieldCheck,
} from 'lucide-react'
import type { PropsWithChildren } from 'react'

import { cn } from '@/lib/utils'

const navigationItems = [
  { label: '概览', icon: BarChart3, active: true },
  { label: '知识管理', icon: BookOpen },
  { label: '智能问答', icon: MessageSquareText },
  { label: '报告生成', icon: FileText },
  { label: '权限管理', icon: ShieldCheck },
  { label: '系统设置', icon: Settings },
]

export function AppLayout({ children }: PropsWithChildren) {
  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="主导航">
        <div className="brand">
          <div className="brand-mark">ST</div>
          <div>
            <p className="brand-name">Software Teamwork</p>
            <p className="brand-caption">前端工作台</p>
          </div>
        </div>

        <nav className="nav-list">
          {navigationItems.map((item) => (
            <button
              key={item.label}
              type="button"
              className={cn('nav-item', item.active && 'nav-item-active')}
            >
              <item.icon aria-hidden="true" size={18} />
              <span>{item.label}</span>
            </button>
          ))}
        </nav>
      </aside>

      <div className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">develop</p>
            <h1>技术监督辅助平台</h1>
          </div>
          <div className="status-pill">Bun + React + TypeScript</div>
        </header>

        <main className="content">{children}</main>
      </div>
    </div>
  )
}

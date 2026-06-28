import { AppLayout } from '@/layouts/app-layout'
import { DashboardPage } from '@/pages/dashboard/page'

export function AppRouter() {
  return (
    <AppLayout>
      <DashboardPage />
    </AppLayout>
  )
}

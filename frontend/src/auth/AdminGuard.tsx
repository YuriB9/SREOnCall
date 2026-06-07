import { type ReactNode } from 'react'
import { useParams } from 'react-router-dom'

import { ForbiddenPage } from '@/pages/ForbiddenPage'

import { usePermissions } from './usePermissions'

export function AdminGuard({ children }: { children: ReactNode }) {
  const { tenant } = useParams<{ tenant: string }>()
  const permissions = usePermissions()

  if (!tenant || permissions[tenant] !== 'admin') {
    return <ForbiddenPage message="Требуются права администратора" />
  }

  return <>{children}</>
}

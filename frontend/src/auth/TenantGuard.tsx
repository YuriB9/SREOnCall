import { type ReactNode } from 'react'

import { useParams } from 'react-router-dom'

import { ForbiddenPage } from '@/pages/ForbiddenPage'

import { usePermissions } from './usePermissions'

export function TenantGuard({ children }: { children: ReactNode }) {
  const { tenant } = useParams<{ tenant: string }>()
  const permissions = usePermissions()

  if (!tenant || !(tenant in permissions)) {
    return <ForbiddenPage message="У вас нет доступа к этой команде" />
  }

  return <>{children}</>
}

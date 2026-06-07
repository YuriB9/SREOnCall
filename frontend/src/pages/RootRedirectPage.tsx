import { Navigate } from 'react-router-dom'

import { usePermissions } from '@/auth/usePermissions'

export function RootRedirectPage() {
  const permissions = usePermissions()
  const tenants = Object.keys(permissions)

  if (tenants.length === 1) {
    return <Navigate to={`/${tenants[0]}/incidents`} replace />
  }
  return <Navigate to="/select-team" replace />
}

import { useMemo } from 'react'

import { useAuth } from './AuthProvider'

export type TenantRole = 'member' | 'admin'

// Groups format from Keycloak:
//   "/team-alpha"         → { "team-alpha": "member" }
//   "/team-beta/admins"   → { "team-beta": "admin" }   (overrides member)
export function parseGroups(groups: string[] = []): Record<string, TenantRole> {
  const roles: Record<string, TenantRole> = {}
  for (const group of groups) {
    const parts = group.replace(/^\//, '').split('/')
    if (parts.length === 1 && parts[0]) {
      if (!roles[parts[0]]) roles[parts[0]] = 'member'
    } else if (parts.length === 2 && parts[1] === 'admins' && parts[0]) {
      roles[parts[0]] = 'admin'
    }
  }
  return roles
}

export function usePermissions(): Record<string, TenantRole> {
  const { user } = useAuth()
  return useMemo(() => {
    if (!user) return {}
    const groups = (user.profile['groups'] as string[] | undefined) ?? []
    return parseGroups(groups)
  }, [user])
}

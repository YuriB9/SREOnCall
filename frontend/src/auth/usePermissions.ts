import { useMemo } from 'react'

import { useAuth } from './useAuth'

export type TenantRole = 'member' | 'admin'

// Mirrors backend pkg/auth: IsMember treats "/{tenant}" and any "/{tenant}/..."
// as membership; IsAdmin requires exactly "/{tenant}/admins".
//   "/team-alpha"          → { "team-alpha": "member" }
//   "/team-gamma/oncall"   → { "team-gamma": "member" }
//   "/team-beta/admins"    → { "team-beta": "admin" }   (overrides member)
export function parseGroups(groups: string[] = []): Record<string, TenantRole> {
  const roles: Record<string, TenantRole> = {}
  for (const group of groups) {
    const parts = group.replace(/^\//, '').split('/')
    const tenant = parts[0]
    if (!tenant) continue
    if (parts.length === 2 && parts[1] === 'admins') {
      roles[tenant] = 'admin'
    } else if (!roles[tenant]) {
      roles[tenant] = 'member'
    }
  }
  return roles
}

// The groups claim may be malformed when the Keycloak mapper is misconfigured
// (missing, a plain string instead of an array, non-string members).
function normalizeGroupsClaim(claim: unknown): string[] {
  if (Array.isArray(claim)) return claim.filter((g): g is string => typeof g === 'string')
  if (typeof claim === 'string' && claim) return [claim]
  return []
}

// Raw groups claim from the id_token, for diagnostics: a non-empty claim with
// an empty tenantRoles map points at a Keycloak mapper misconfiguration.
export function useRawGroups(): string[] {
  const { user } = useAuth()
  return useMemo(() => (user ? normalizeGroupsClaim(user.profile['groups']) : []), [user])
}

export function usePermissions(): Record<string, TenantRole> {
  const { user } = useAuth()
  return useMemo(() => {
    if (!user) return {}
    return parseGroups(normalizeGroupsClaim(user.profile['groups']))
  }, [user])
}

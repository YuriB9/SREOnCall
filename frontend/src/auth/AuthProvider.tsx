import type { User } from 'oidc-client-ts'
import { type ReactNode,useCallback, useEffect, useState } from 'react'

import { AuthContext } from './authContext'
import { userManager } from './oidcConfig'

// Dev-only mock: set VITE_MOCK_AUTH_GROUPS=/team-alpha,/team-alpha/admins in .env.local
function buildMockUser(): User | null {
  const groups = import.meta.env.VITE_MOCK_AUTH_GROUPS
  if (!import.meta.env.DEV || !groups) return null
  return {
    profile: {
      sub: 'dev-user',
      preferred_username: 'dev',
      groups: groups.split(',').map((g: string) => g.trim()),
    },
    access_token: 'mock-token',
    token_type: 'Bearer',
    scope: 'openid profile email',
    expired: false,
  } as unknown as User
}

// Computed once at module load — env vars are static
const MOCK_USER = buildMockUser()

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(MOCK_USER)
  const [loading, setLoading] = useState(MOCK_USER === null)

  useEffect(() => {
    if (MOCK_USER) return // skip OIDC in mock mode

    userManager.getUser().then((u) => {
      setUser(u)
      setLoading(false)
    })

    const onUserLoaded = (u: User) => setUser(u)
    const onUserUnloaded = () => setUser(null)

    userManager.events.addUserLoaded(onUserLoaded)
    userManager.events.addUserUnloaded(onUserUnloaded)

    return () => {
      userManager.events.removeUserLoaded(onUserLoaded)
      userManager.events.removeUserUnloaded(onUserUnloaded)
    }
  }, [])

  const signIn = useCallback(
    () =>
      userManager.signinRedirect({
        state: window.location.pathname + window.location.search + window.location.hash,
      }),
    [],
  )

  const signOut = useCallback(() => userManager.signoutRedirect(), [])

  return (
    <AuthContext.Provider value={{ user, loading, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  )
}

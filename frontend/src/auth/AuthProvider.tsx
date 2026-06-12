import type { User } from 'oidc-client-ts'
import { type ReactNode,useCallback, useEffect, useRef, useState } from 'react'

import { syncUserContacts } from '@/api/profile'

import { AuthContext } from './authContext'
import { logoutInProgress, OIDC_CLIENT_ID, userManager } from './oidcConfig'

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
  // Tracks which user subs we've already synced this session to avoid
  // re-syncing on every userLoaded event (e.g. silent token renewals).
  const syncedSub = useRef<string | null>(null)

  useEffect(() => {
    if (MOCK_USER) return // skip OIDC in mock mode

    // Provision/refresh the user's contacts from the Keycloak token on login.
    const syncContacts = (u: User | null) => {
      const sub = u?.profile.sub
      if (!sub || !u?.access_token || syncedSub.current === sub) return
      syncedSub.current = sub
      syncUserContacts().catch(() => {
        // Best-effort: a failed sync shouldn't block the app; the user can
        // still configure contacts manually on the profile page.
        syncedSub.current = null
      })
    }

    userManager.getUser().then((u) => {
      setUser(u)
      setLoading(false)
      syncContacts(u)
    })

    const onUserLoaded = (u: User) => {
      setUser(u)
      syncContacts(u)
    }
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

  const signOut = useCallback(async () => {
    // Помечаем выход до любых вызовов userManager: signoutRedirect удалит
    // пользователя из хранилища и синхронно стрельнёт userUnloaded, иначе
    // RequireAuth перехватит это и запустит вход, обогнав logout-навигацию.
    logoutInProgress.current = true
    try {
      // Keycloak отклоняет logout с кодом 400, если id_token_hint протух — при
      // этом SSO-сессия не завершается, и RequireAuth тут же логинит обратно.
      // automaticSilentRenew обычно держит токен свежим, но если он перестал
      // работать (истёк refresh-токен, простой сессии) — хинт оказывается
      // протухшим. Поэтому перед выходом обновляем токен.
      const current = await userManager.getUser()
      if (current?.expired) {
        try {
          await userManager.signinSilent()
        } catch {
          // Refresh-токен тоже мёртв: убираем протухшего пользователя, чтобы не
          // отправлять невалидный id_token_hint (иначе снова 400), и выходим по
          // client_id — Keycloak покажет страницу подтверждения вместо отказа.
          await userManager.removeUser()
          await userManager.signoutRedirect({
            extraQueryParams: { client_id: OIDC_CLIENT_ID },
          })
          return
        }
      }
      await userManager.signoutRedirect()
    } catch (err) {
      // Не удалось уйти на logout (например, Keycloak недоступен) — снимаем
      // флаг, иначе RequireAuth останется заблокированным на пустом экране.
      logoutInProgress.current = false
      console.error('AuthProvider: signOut failed:', err)
    }
  }, [])

  return (
    <AuthContext.Provider value={{ user, loading, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  )
}

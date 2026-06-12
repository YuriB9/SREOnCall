import { type ReactNode, useCallback, useEffect, useState } from 'react'

import { AuthErrorScreen } from './AuthErrorScreen'
import { logoutInProgress } from './oidcConfig'
import { useAuth } from './useAuth'

export function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading, signIn } = useAuth()
  const [signInFailed, setSignInFailed] = useState(false)

  const attemptSignIn = useCallback(() => {
    // signinRedirect обычно уводит со страницы; reject означает, что Keycloak
    // недоступен и пользователь остался бы на пустом экране.
    signIn().catch((err: unknown) => {
      console.error('RequireAuth: signinRedirect failed:', err)
      setSignInFailed(true)
    })
  }, [signIn])

  useEffect(() => {
    // logoutInProgress: во время выхода signoutRedirect удаляет пользователя
    // до навигации; без этой проверки мы запустили бы вход и перехватили
    // logout, молча залогинив обратно.
    if (!loading && !user && !signInFailed && !logoutInProgress.current) {
      attemptSignIn()
    }
  }, [loading, user, signInFailed, attemptSignIn])

  if (signInFailed) {
    return (
      <AuthErrorScreen
        title="Сервис авторизации недоступен"
        message="Не удалось связаться с сервером входа. Проверьте соединение и повторите попытку позже."
        actionLabel="Повторить попытку"
        // Сброс флага перезапускает эффект входа выше.
        onAction={() => setSignInFailed(false)}
      />
    )
  }

  if (loading || !user) return null

  return <>{children}</>
}

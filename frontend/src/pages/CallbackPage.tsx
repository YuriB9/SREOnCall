import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { AuthErrorScreen } from '@/auth/AuthErrorScreen'
import { userManager } from '@/auth/oidcConfig'

// Повтор входа с /callback: state указывает на корень, а не на текущий URL —
// возврат на /callback со старыми параметрами снова дал бы ошибку обмена.
function retrySignIn() {
  userManager
    .signinRedirect({ state: '/' })
    .catch((err: unknown) => console.error('CallbackPage: retry signinRedirect failed:', err))
}

export function CallbackPage() {
  const navigate = useNavigate()
  const calledRef = useRef(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (calledRef.current) return
    calledRef.current = true

    userManager
      .signinRedirectCallback()
      .then((user) => {
        const destination = typeof user.state === 'string' ? user.state : '/'
        navigate(destination, { replace: true })
      })
      .catch((err: unknown) => {
        // Не редиректим на '/': это снова запустило бы RequireAuth → signIn
        // и зациклило бы редиректы при недоступном Keycloak.
        console.error('CallbackPage: signinRedirectCallback failed:', err)
        setFailed(true)
      })
  }, [navigate])

  if (failed) {
    return (
      <AuthErrorScreen
        title="Не удалось завершить вход"
        message="Сервис авторизации вернул ошибку или недоступен. Попробуйте войти ещё раз."
        actionLabel="Повторить вход"
        onAction={retrySignIn}
      />
    )
  }

  return null
}

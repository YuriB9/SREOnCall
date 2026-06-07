import { type ReactNode, useEffect } from 'react'

import { useAuth } from './AuthProvider'

export function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading, signIn } = useAuth()

  useEffect(() => {
    if (!loading && !user) {
      signIn()
    }
  }, [loading, user, signIn])

  if (loading || !user) return null

  return <>{children}</>
}

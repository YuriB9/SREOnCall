import { useEffect } from 'react'

import { useNavigate } from 'react-router-dom'

import { userManager } from '@/auth/oidcConfig'

export function CallbackPage() {
  const navigate = useNavigate()

  useEffect(() => {
    userManager
      .signinRedirectCallback()
      .then((user) => {
        const destination = typeof user.state === 'string' ? user.state : '/'
        navigate(destination, { replace: true })
      })
      .catch(() => navigate('/', { replace: true }))
  }, [navigate])

  return null
}

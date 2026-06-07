import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'

import { userManager } from '@/auth/oidcConfig'

export function CallbackPage() {
  const navigate = useNavigate()
  const calledRef = useRef(false)

  useEffect(() => {
    if (calledRef.current) return
    calledRef.current = true

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

import { useEffect } from 'react'

import { userManager } from '@/auth/oidcConfig'

// Loaded inside a hidden iframe by oidc-client-ts to complete silent token renewal.
export function SilentRenewPage() {
  useEffect(() => {
    userManager.signinSilentCallback().catch(() => {})
  }, [])

  return null
}

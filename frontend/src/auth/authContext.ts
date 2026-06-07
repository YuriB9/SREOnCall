import type { User } from 'oidc-client-ts'
import { createContext } from 'react'

export interface AuthContextValue {
  user: User | null
  loading: boolean
  signIn: () => Promise<void>
  signOut: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | null>(null)

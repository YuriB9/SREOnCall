/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_OIDC_AUTHORITY: string
  readonly VITE_OIDC_CLIENT_ID: string
  // Dev only — bypasses OIDC when set
  readonly VITE_MOCK_AUTH_GROUPS?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

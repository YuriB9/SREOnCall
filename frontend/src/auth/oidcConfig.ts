import { UserManager, WebStorageStateStore } from 'oidc-client-ts'

export const OIDC_CLIENT_ID = import.meta.env.VITE_OIDC_CLIENT_ID

// Выставляется на время logout-редиректа. signoutRedirect() удаляет
// пользователя из хранилища ДО навигации, что синхронно стреляет userUnloaded;
// без этого флага RequireAuth тут же запустил бы signinRedirect и выиграл бы
// гонку у logout-навигации — пользователя молча залогинило бы обратно. Это
// обычный объект (не useState), чтобы читаться синхронно в эффекте RequireAuth.
export const logoutInProgress = { current: false }

export const userManager = new UserManager({
  authority: import.meta.env.VITE_OIDC_AUTHORITY,
  client_id: OIDC_CLIENT_ID,
  redirect_uri: `${window.location.origin}/callback`,
  // Без post_logout_redirect_uri запрос на logout к Keycloak не завершает
  // SSO-сессию, и RequireAuth тут же логинит пользователя обратно.
  post_logout_redirect_uri: window.location.origin,
  scope: 'openid profile email',
  userStore: new WebStorageStateStore({ store: window.sessionStorage }),
  accessTokenExpiringNotificationTimeInSeconds: 120,
  automaticSilentRenew: true,
  silent_redirect_uri: `${window.location.origin}/silent-renew`,
})

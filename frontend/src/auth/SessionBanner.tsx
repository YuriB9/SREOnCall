import { useEffect, useRef, useState } from 'react'

import { userManager } from './oidcConfig'
import { useAuth } from './useAuth'

const COUNTDOWN_SECONDS = 30
// Порог близости истечения токена; совпадает с
// accessTokenExpiringNotificationTimeInSeconds в oidcConfig.ts.
const EXPIRY_THRESHOLD_SECONDS = 120
// Сколько подряд неудачных тихих обновлений терпим, пока токен ещё валиден.
const MAX_CONSECUTIVE_RENEW_FAILURES = 3

export function SessionBanner() {
  const [visible, setVisible] = useState(false)
  const [secondsLeft, setSecondsLeft] = useState(COUNTDOWN_SECONDS)
  const { signIn } = useAuth()
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const visibleRef = useRef(false)
  const failuresRef = useRef(0)

  useEffect(() => {
    function clearTimers() {
      if (timerRef.current) clearTimeout(timerRef.current)
      if (intervalRef.current) clearInterval(intervalRef.current)
    }

    function startBanner() {
      if (visibleRef.current) return
      visibleRef.current = true
      setVisible(true)
      setSecondsLeft(COUNTDOWN_SECONDS)

      intervalRef.current = setInterval(() => {
        setSecondsLeft((s) => Math.max(0, s - 1))
      }, 1000)

      timerRef.current = setTimeout(() => {
        signIn()
      }, COUNTDOWN_SECONDS * 1000)
    }

    function hideBanner() {
      failuresRef.current = 0
      if (!visibleRef.current) return
      visibleRef.current = false
      setVisible(false)
      clearTimers()
    }

    // Единичный транзиентный сбой renew при ещё валидном токене не должен
    // пугать дежурного отсчётом до выхода. Баннер запускается, только когда
    // до истечения токена осталось меньше порога либо сбои стали системными.
    function onSilentRenewError(err: unknown) {
      failuresRef.current += 1
      void userManager.getUser().then((user) => {
        const expiresIn = user?.expires_in
        const nearExpiry = expiresIn === undefined || expiresIn < EXPIRY_THRESHOLD_SECONDS
        if (nearExpiry || failuresRef.current >= MAX_CONSECUTIVE_RENEW_FAILURES) {
          startBanner()
        } else {
          console.warn(
            `SessionBanner: silent renew failed (${failuresRef.current}/${MAX_CONSECUTIVE_RENEW_FAILURES}), ` +
              `token expires in ${expiresIn}s — баннер не показан:`,
            err,
          )
        }
      })
    }

    userManager.events.addSilentRenewError(onSilentRenewError)
    userManager.events.addUserLoaded(hideBanner)

    return () => {
      userManager.events.removeSilentRenewError(onSilentRenewError)
      userManager.events.removeUserLoaded(hideBanner)
      clearTimers()
    }
  }, [signIn])

  if (!visible) return null

  return (
    <div
      role="alert"
      className="fixed inset-x-0 top-0 z-50 flex items-center justify-between bg-yellow-500 px-4 py-2 text-sm font-medium text-yellow-950"
    >
      <span>Ваша сессия истекает. Вы будете выведены из системы через {secondsLeft} сек.</span>
      <button
        onClick={signIn}
        className="ml-4 rounded bg-yellow-950 px-3 py-1 text-yellow-50 hover:bg-yellow-900"
      >
        Войти снова
      </button>
    </div>
  )
}

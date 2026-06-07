import { useEffect, useRef, useState } from 'react'

import { userManager } from './oidcConfig'
import { useAuth } from './useAuth'

const COUNTDOWN_SECONDS = 30

export function SessionBanner() {
  const [visible, setVisible] = useState(false)
  const [secondsLeft, setSecondsLeft] = useState(COUNTDOWN_SECONDS)
  const { signIn } = useAuth()
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    function clearTimers() {
      if (timerRef.current) clearTimeout(timerRef.current)
      if (intervalRef.current) clearInterval(intervalRef.current)
    }

    function startBanner() {
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
      setVisible(false)
      clearTimers()
    }

    userManager.events.addSilentRenewError(startBanner)
    userManager.events.addUserLoaded(hideBanner)

    return () => {
      userManager.events.removeSilentRenewError(startBanner)
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

import { useCallback, useState } from 'react'

export function useAudioEnabled() {
  const [enabled, setEnabled] = useState(
    () => localStorage.getItem('oncall.audioEnabled') !== 'false',
  )

  const set = useCallback((value: boolean) => {
    setEnabled(value)
    localStorage.setItem('oncall.audioEnabled', String(value))
  }, [])

  return [enabled, set] as const
}

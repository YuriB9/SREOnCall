import { useCallback, useState } from 'react'

type ColorScheme = 'dark' | 'light'

export function useTheme() {
  const [scheme, setScheme] = useState<ColorScheme>(
    () => (localStorage.getItem('oncall.colorScheme') as ColorScheme) ?? 'dark',
  )

  const toggle = useCallback(() => {
    setScheme((current) => {
      const next: ColorScheme = current === 'dark' ? 'light' : 'dark'
      localStorage.setItem('oncall.colorScheme', next)
      document.documentElement.classList.toggle('dark', next === 'dark')
      return next
    })
  }, [])

  return { scheme, toggle }
}

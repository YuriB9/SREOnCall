import { useEffect, useRef } from 'react'

type KeyMap = Record<string, () => void>

const BLOCKED_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

export function useKeyMap(bindings: KeyMap) {
  const ref = useRef(bindings)
  ref.current = bindings

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const target = e.target as HTMLElement
      if (BLOCKED_TAGS.has(target.tagName) || target.isContentEditable) return
      if (e.ctrlKey || e.metaKey || e.altKey) return

      const handler = ref.current[e.key]
      if (handler) {
        e.preventDefault()
        handler()
      }
    }

    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])
}

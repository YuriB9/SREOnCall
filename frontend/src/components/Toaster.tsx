import { useEffect, useState } from 'react'

import { _registerToastFn } from '@/lib/toast'
import { cn } from '@/lib/utils'

interface ToastItem {
  id: number
  message: string
  type: 'error' | 'success'
}

let _nextId = 0

export function Toaster() {
  const [toasts, setToasts] = useState<ToastItem[]>([])

  useEffect(() => {
    _registerToastFn((message, type = 'error') => {
      const id = _nextId++
      setToasts((prev) => [...prev, { id, message, type }])
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id))
      }, 4000)
    })
    return () => _registerToastFn(null)
  }, [])

  if (toasts.length === 0) return null

  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={cn(
            'pointer-events-auto rounded-lg border px-4 py-3 text-sm shadow-lg',
            t.type === 'error'
              ? 'border-destructive/30 bg-background text-destructive'
              : 'border-green-500/30 bg-background text-green-700 dark:text-green-400',
          )}
        >
          {t.message}
        </div>
      ))}
    </div>
  )
}

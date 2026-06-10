import { Button } from '@/components/ui/button'

// Полноэкранная заглушка для ошибок аутентификации (Keycloak недоступен,
// сбой обмена на /callback). Без автоматических редиректов — только явное
// действие пользователя, чтобы не создавать цикл редиректов.
export function AuthErrorScreen({
  title,
  message,
  actionLabel,
  onAction,
}: {
  title: string
  message: string
  actionLabel: string
  onAction: () => void
}) {
  return (
    <div className="flex h-screen flex-col items-center justify-center gap-3">
      <p className="text-2xl font-bold">{title}</p>
      <p className="max-w-md text-center text-muted-foreground">{message}</p>
      <Button className="mt-2" onClick={onAction}>
        {actionLabel}
      </Button>
    </div>
  )
}

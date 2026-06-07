import { Link } from 'react-router-dom'

interface Props {
  message?: string
}

export function ForbiddenPage({ message = 'У вас нет доступа к этой странице' }: Props) {
  return (
    <div className="flex h-screen flex-col items-center justify-center gap-4">
      <p className="text-6xl font-bold text-muted-foreground">403</p>
      <p className="text-lg">{message}</p>
      <Link to="/" className="text-sm text-primary underline underline-offset-4">
        На главную
      </Link>
    </div>
  )
}

import { Component, type ReactNode } from 'react'

// Последний рубеж для непредвиденных исключений рендера: вместо белого экрана —
// сообщение и перезагрузка. Ошибки промисов (OIDC) обрабатываются точечно
// в RequireAuth/CallbackPage и сюда не попадают.
export class AppErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  state = { hasError: false }

  static getDerivedStateFromError() {
    return { hasError: true }
  }

  componentDidCatch(error: unknown, info: unknown) {
    console.error('AppErrorBoundary: unhandled render error:', error, info)
  }

  render() {
    if (!this.state.hasError) return this.props.children
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-3">
        <p className="text-2xl font-bold">Что-то пошло не так</p>
        <p className="max-w-md text-center text-muted-foreground">
          Произошла непредвиденная ошибка. Перезагрузите страницу; если ошибка повторяется —
          обратитесь к администратору.
        </p>
        <button
          className="mt-2 rounded-md border px-4 py-2 text-sm hover:bg-accent"
          onClick={() => window.location.reload()}
        >
          Перезагрузить
        </button>
      </div>
    )
  }
}

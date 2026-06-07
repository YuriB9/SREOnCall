import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

import { usePermissions } from '@/auth/usePermissions'
import { Button } from '@/components/ui/button'

export function SelectTeamPage() {
  const permissions = usePermissions()
  const navigate = useNavigate()
  const tenants = Object.keys(permissions)

  useEffect(() => {
    if (tenants.length === 1) {
      navigate(`/${tenants[0]}/incidents`, { replace: true })
    }
  }, [tenants, navigate])

  if (tenants.length === 0) {
    return (
      <div className="flex h-screen flex-col items-center justify-center gap-3">
        <p className="text-2xl font-bold">Нет доступа к командам</p>
        <p className="text-muted-foreground">
          У вас нет членства ни в одной команде. Обратитесь к администратору.
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-screen flex-col items-center justify-center gap-6">
      <p className="text-2xl font-bold">Выберите команду</p>
      <ul className="flex flex-col gap-2">
        {tenants.map((slug) => (
          <li key={slug}>
            <Button
              variant="outline"
              className="w-52 justify-between"
              onClick={() => navigate(`/${slug}/incidents`)}
            >
              <span>{slug}</span>
              {permissions[slug] === 'admin' && (
                <span className="text-xs text-muted-foreground">admin</span>
              )}
            </Button>
          </li>
        ))}
      </ul>
    </div>
  )
}

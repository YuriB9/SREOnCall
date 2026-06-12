import { Info, Users } from 'lucide-react'
import { useParams } from 'react-router-dom'

import { useMembers } from '@/api/tenantSettings'
import type { Member } from '@/api/types'

function roleLabel(role: string) {
  return role === 'admin' ? 'Администратор' : 'Участник'
}

// ── MembersSection ────────────────────────────────────────────────────────────

export function MembersSection({ tenant }: { tenant: string }) {
  const { data: rawMembers, isLoading } = useMembers(tenant)
  const members: Member[] = Array.isArray(rawMembers) ? rawMembers : []

  return (
    <section className="rounded-lg border p-5">
      <div className="mb-3 flex items-center gap-2">
        <Users size={16} className="text-muted-foreground" />
        <h2 className="text-sm font-semibold">Участники команды</h2>
      </div>

      <div className="mb-4 flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-3 py-2.5 text-xs text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-300">
        <Info size={13} className="mt-0.5 shrink-0" />
        <span>
          Состав команды управляется в Keycloak. Для добавления или удаления участников используйте консоль администратора Keycloak.
        </span>
      </div>

      {isLoading ? (
        <p className="py-6 text-center text-xs text-muted-foreground">Загрузка...</p>
      ) : members.length === 0 ? (
        <p className="py-6 text-center text-xs text-muted-foreground">Нет участников.</p>
      ) : (
        <div className="flex flex-col divide-y">
          {members.map((m) => (
            <div key={m.user_id} className="flex items-center justify-between py-2.5">
              <p className="text-sm">{m.preferred_username}</p>
              <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                {roleLabel(m.role)}
              </span>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

// ── MembersPage ───────────────────────────────────────────────────────────────

export function MembersPage() {
  const { tenant } = useParams<{ tenant: string }>()

  return (
    <div className="flex h-full flex-col gap-5 overflow-auto p-4">
      <h1 className="text-base font-semibold">Участники команды</h1>
      <MembersSection tenant={tenant!} />
    </div>
  )
}

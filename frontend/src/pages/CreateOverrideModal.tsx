import { format } from 'date-fns'
import { X } from 'lucide-react'
import { useState } from 'react'

import { OverrideConflictError, useCreateOverride } from '@/api/schedules'
import type { Member, Schedule } from '@/api/types'
import { Button } from '@/components/ui/button'

interface Props {
  tenant: string
  schedules: Schedule[]
  members: Member[]
  initialScheduleId?: string
  onClose: () => void
}

export function CreateOverrideModal({
  tenant,
  schedules,
  members,
  initialScheduleId,
  onClose,
}: Props) {
  const [scheduleId, setScheduleId] = useState(initialScheduleId ?? schedules[0]?.id ?? '')
  const [userId, setUserId] = useState('')
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [conflictError, setConflictError] = useState<string | null>(null)

  const createOverride = useCreateOverride(tenant)

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setConflictError(null)
    createOverride.mutate(
      {
        scheduleId,
        user_id: userId,
        start_at: new Date(start).toISOString(),
        end_at: new Date(end).toISOString(),
      },
      {
        onSuccess: onClose,
        onError: (err) => {
          if (err instanceof OverrideConflictError) {
            const d = err.detail
            setConflictError(
              `Конфликт с существующей заменой: ${format(new Date(d.existing_start), 'dd.MM HH:mm')} — ${format(new Date(d.existing_end), 'dd.MM HH:mm')} (${d.existing_user})`,
            )
          } else {
            setConflictError('Не удалось создать замену')
          }
        },
      },
    )
  }

  const canSubmit = Boolean(scheduleId && userId && start && end) && !createOverride.isPending

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-5 flex items-center justify-between">
          <h2 className="text-base font-semibold">Создать замену</h2>
          <button onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Schedule */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Расписание</label>
            <select
              value={scheduleId}
              onChange={(e) => setScheduleId(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              {schedules.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </div>

          {/* Member */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Замещающий</label>
            <select
              value={userId}
              onChange={(e) => setUserId(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">Выберите инженера</option>
              {members.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  @{m.preferred_username}
                </option>
              ))}
            </select>
          </div>

          {/* Start */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Начало</label>
            <input
              type="datetime-local"
              value={start}
              onChange={(e) => setStart(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* End */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Конец</label>
            <input
              type="datetime-local"
              value={end}
              onChange={(e) => setEnd(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* Inline conflict error (task 6.8) */}
          {conflictError && (
            <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {conflictError}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              Отмена
            </Button>
            <Button type="submit" size="sm" disabled={!canSubmit}>
              {createOverride.isPending ? 'Создание...' : 'Создать'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

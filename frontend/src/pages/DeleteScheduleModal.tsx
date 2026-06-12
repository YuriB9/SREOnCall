import { X } from 'lucide-react'
import { useState } from 'react'

import { useDeleteSchedule } from '@/api/schedules'
import type { Schedule } from '@/api/types'
import { Button } from '@/components/ui/button'

interface Props {
  tenant: string
  schedule: Schedule
  onClose: () => void
}

export function DeleteScheduleModal({ tenant, schedule, onClose }: Props) {
  const deleteSchedule = useDeleteSchedule(tenant)
  const [error, setError] = useState<string | null>(null)

  function handleDelete() {
    setError(null)
    deleteSchedule.mutate(schedule.id, {
      onSuccess: onClose,
      onError: () => setError('Не удалось удалить расписание'),
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">Удалить расписание</h2>
          <button onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X size={16} />
          </button>
        </div>

        <p className="text-sm text-muted-foreground">
          Вы уверены, что хотите удалить расписание{' '}
          <span className="font-medium text-foreground">«{schedule.name}»</span>? Действие
          необратимо.
        </p>

        {error && (
          <p className="mt-4 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {error}
          </p>
        )}

        <div className="mt-6 flex justify-end gap-2">
          <Button type="button" variant="outline" size="sm" onClick={onClose}>
            Отмена
          </Button>
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={deleteSchedule.isPending}
            onClick={handleDelete}
          >
            {deleteSchedule.isPending ? 'Удаление...' : 'Удалить'}
          </Button>
        </div>
      </div>
    </div>
  )
}

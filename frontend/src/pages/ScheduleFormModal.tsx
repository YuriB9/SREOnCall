import { format } from 'date-fns'
import { ArrowDown, ArrowUp, Plus, X } from 'lucide-react'
import { useState } from 'react'

import {
  OverrideConflictError,
  ScheduleValidationError,
  useCreateOverride,
  useCreateSchedule,
  useUpdateSchedule,
} from '@/api/schedules'
import type { Member, Schedule } from '@/api/types'
import { Button } from '@/components/ui/button'

interface Props {
  tenant: string
  members: Member[]
  schedule?: Schedule
  onClose: () => void
}

const DURATION_PRESETS: { value: string; label: string }[] = [
  { value: 'P1D', label: '1 день' },
  { value: 'P7D', label: '7 дней' },
  { value: 'P14D', label: '14 дней' },
  { value: 'PT8H', label: '8 часов' },
  { value: 'PT12H', label: '12 часов' },
]

// Common IANA timezones; backend uses the value as-is, default UTC.
const TIMEZONES = [
  'UTC',
  'Europe/Moscow',
  'Europe/London',
  'Europe/Berlin',
  'America/New_York',
  'America/Los_Angeles',
  'Asia/Yekaterinburg',
  'Asia/Novosibirsk',
  'Asia/Tokyo',
]

function todayISODate(): string {
  return new Date().toISOString().slice(0, 10)
}

export function ScheduleFormModal({ tenant, members, schedule, onClose }: Props) {
  const isEdit = Boolean(schedule)

  const [name, setName] = useState(schedule?.name ?? '')
  const [timezone, setTimezone] = useState(schedule?.timezone || 'UTC')
  const [shiftDuration, setShiftDuration] = useState(schedule?.shift_duration ?? 'P7D')
  const [startDate, setStartDate] = useState(schedule?.start_date?.slice(0, 10) ?? todayISODate())
  const [rotation, setRotation] = useState<string[]>(schedule?.rotation ?? [])
  const [addUserId, setAddUserId] = useState('')
  const [error, setError] = useState<string | null>(null)

  const createSchedule = useCreateSchedule(tenant)
  const updateSchedule = useUpdateSchedule(tenant)
  const isPending = createSchedule.isPending || updateSchedule.isPending

  // ── Override (replacement) creation — edit mode only ────────────────────────
  const createOverride = useCreateOverride(tenant)
  const [ovrUserId, setOvrUserId] = useState('')
  const [ovrStart, setOvrStart] = useState('')
  const [ovrEnd, setOvrEnd] = useState('')
  const [ovrError, setOvrError] = useState<string | null>(null)
  const [ovrSuccess, setOvrSuccess] = useState<string | null>(null)

  const canSubmitOverride =
    Boolean(ovrUserId && ovrStart && ovrEnd) && !createOverride.isPending

  function handleCreateOverride() {
    if (!schedule) return
    setOvrError(null)
    setOvrSuccess(null)
    createOverride.mutate(
      {
        scheduleId: schedule.id,
        user_id: ovrUserId,
        start_at: new Date(ovrStart).toISOString(),
        end_at: new Date(ovrEnd).toISOString(),
      },
      {
        onSuccess: () => {
          setOvrSuccess('Замена создана')
          setOvrUserId('')
          setOvrStart('')
          setOvrEnd('')
        },
        onError: (err) => {
          if (err instanceof OverrideConflictError) {
            const d = err.detail
            const member = members.find((m) => m.user_id === d.existing_user)
            const who = member ? `@${member.preferred_username}` : d.existing_user
            setOvrError(
              `Конфликт с существующей заменой: ${format(new Date(d.existing_start), 'dd.MM HH:mm')} — ${format(new Date(d.existing_end), 'dd.MM HH:mm')} (${who})`,
            )
          } else {
            setOvrError('Не удалось создать замену')
          }
        },
      },
    )
  }

  const usernameOf = (userId: string) =>
    members.find((m) => m.user_id === userId)?.preferred_username ?? userId.slice(0, 8)

  const available = members.filter((m) => !rotation.includes(m.user_id))

  function addToRotation() {
    if (addUserId && !rotation.includes(addUserId)) {
      setRotation((r) => [...r, addUserId])
      setAddUserId('')
    }
  }

  function removeFromRotation(userId: string) {
    setRotation((r) => r.filter((u) => u !== userId))
  }

  function move(index: number, delta: number) {
    setRotation((r) => {
      const next = [...r]
      const target = index + delta
      if (target < 0 || target >= next.length) return r
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)

    const input = {
      name: name.trim(),
      timezone,
      rotation,
      shift_duration: shiftDuration,
      start_date: startDate,
    }

    const onError = (err: unknown) => {
      if (err instanceof ScheduleValidationError) {
        const fields = err.missingFields?.length
          ? ` (${err.missingFields.join(', ')})`
          : ''
        setError(`${err.message}${fields}`)
      } else {
        setError(isEdit ? 'Не удалось сохранить расписание' : 'Не удалось создать расписание')
      }
    }

    if (isEdit && schedule) {
      updateSchedule.mutate({ id: schedule.id, ...input }, { onSuccess: onClose, onError })
    } else {
      createSchedule.mutate(input, { onSuccess: onClose, onError })
    }
  }

  const canSubmit =
    Boolean(name.trim() && shiftDuration && startDate) && rotation.length > 0 && !isPending

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-5 flex items-center justify-between">
          <h2 className="text-base font-semibold">
            {isEdit ? 'Редактировать расписание' : 'Создать расписание'}
          </h2>
          <button onClick={onClose} className="rounded p-1 text-muted-foreground hover:bg-muted">
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Name */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Название</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Например, Primary On-Call"
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* Timezone */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Часовой пояс</label>
            <select
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              {TIMEZONES.map((tz) => (
                <option key={tz} value={tz}>
                  {tz}
                </option>
              ))}
            </select>
          </div>

          {/* Shift duration */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Продолжительность смены</label>
            <select
              value={shiftDuration}
              onChange={(e) => setShiftDuration(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              {DURATION_PRESETS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>

          {/* Start date */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Дата старта ротации</label>
            <input
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* Rotation editor */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Ротация инженеров</label>
            <div className="flex gap-2">
              <select
                value={addUserId}
                onChange={(e) => setAddUserId(e.target.value)}
                className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
              >
                <option value="">Добавить инженера…</option>
                {available.map((m) => (
                  <option key={m.user_id} value={m.user_id}>
                    @{m.preferred_username}
                  </option>
                ))}
              </select>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!addUserId}
                onClick={addToRotation}
              >
                <Plus size={14} />
              </Button>
            </div>

            {rotation.length === 0 ? (
              <p className="pt-1 text-xs text-muted-foreground">
                Добавьте хотя бы одного инженера. Порядок определяет очередь дежурств.
              </p>
            ) : (
              <ol className="mt-1 space-y-1">
                {rotation.map((userId, i) => (
                  <li
                    key={userId}
                    className="flex items-center gap-2 rounded-md border px-2 py-1 text-sm"
                  >
                    <span className="w-5 shrink-0 text-xs text-muted-foreground">{i + 1}.</span>
                    <span className="flex-1 truncate">@{usernameOf(userId)}</span>
                    <button
                      type="button"
                      onClick={() => move(i, -1)}
                      disabled={i === 0}
                      title="Вверх"
                      className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
                    >
                      <ArrowUp size={13} />
                    </button>
                    <button
                      type="button"
                      onClick={() => move(i, 1)}
                      disabled={i === rotation.length - 1}
                      title="Вниз"
                      className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
                    >
                      <ArrowDown size={13} />
                    </button>
                    <button
                      type="button"
                      onClick={() => removeFromRotation(userId)}
                      title="Удалить"
                      className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <X size={13} />
                    </button>
                  </li>
                ))}
              </ol>
            )}
          </div>

          {/* Inline validation error (422) */}
          {error && (
            <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {error}
            </p>
          )}

          {/* Override (replacement) editor — edit mode only */}
          {isEdit && (
            <div className="space-y-2 rounded-md border border-dashed p-3">
              <p className="text-xs font-medium text-muted-foreground">Создать замену</p>
              <div className="space-y-1">
                <label className="text-xs text-muted-foreground">Замещающий</label>
                <select
                  value={ovrUserId}
                  onChange={(e) => setOvrUserId(e.target.value)}
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
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1">
                  <label className="text-xs text-muted-foreground">Начало</label>
                  <input
                    type="datetime-local"
                    value={ovrStart}
                    onChange={(e) => setOvrStart(e.target.value)}
                    className="w-full rounded-md border bg-background px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs text-muted-foreground">Конец</label>
                  <input
                    type="datetime-local"
                    value={ovrEnd}
                    onChange={(e) => setOvrEnd(e.target.value)}
                    className="w-full rounded-md border bg-background px-2 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
                  />
                </div>
              </div>
              {ovrError && (
                <p className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {ovrError}
                </p>
              )}
              {ovrSuccess && (
                <p className="rounded-md bg-emerald-500/10 px-3 py-2 text-xs text-emerald-600 dark:text-emerald-400">
                  {ovrSuccess}
                </p>
              )}
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!canSubmitOverride}
                onClick={handleCreateOverride}
              >
                <Plus size={14} className="mr-1" />
                {createOverride.isPending ? 'Создание замены...' : 'Добавить замену'}
              </Button>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="outline" size="sm" onClick={onClose}>
              Отмена
            </Button>
            <Button type="submit" size="sm" disabled={!canSubmit}>
              {isPending
                ? isEdit
                  ? 'Сохранение...'
                  : 'Создание...'
                : isEdit
                  ? 'Сохранить'
                  : 'Создать'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

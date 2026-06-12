import { useQueries } from '@tanstack/react-query'
import {
  addDays,
  addMonths,
  format,
  getDaysInMonth,
  isToday,
  startOfDay,
  startOfMonth,
} from 'date-fns'
import { ru } from 'date-fns/locale'
import { ChevronLeft, ChevronRight, Pencil, Plus, Trash2, User } from 'lucide-react'
import { useState } from 'react'
import { useParams } from 'react-router-dom'

import { apiClient } from '@/api/client'
import { scheduleKeys, useSchedules } from '@/api/schedules'
import { useMembers } from '@/api/tenantSettings'
import type { OnCallNow, Schedule, ShiftWindow } from '@/api/types'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { DeleteScheduleModal } from './DeleteScheduleModal'
import { ScheduleFormModal } from './ScheduleFormModal'

// ── Color palette for shift bars (deterministic per user_id) ─────────────────

const BAR_COLORS = [
  'bg-blue-500',
  'bg-emerald-500',
  'bg-violet-500',
  'bg-amber-500',
  'bg-rose-500',
  'bg-teal-500',
  'bg-indigo-500',
  'bg-orange-500',
]

function userBarColor(userId: string): string {
  let h = 0
  for (const c of userId) h = (h * 31 + c.charCodeAt(0)) & 0xffff
  return BAR_COLORS[h % BAR_COLORS.length]
}

// ── Gantt positioning ─────────────────────────────────────────────────────────

function barPosition(
  shiftStart: Date,
  shiftEnd: Date,
  monthStart: Date,
  monthEndExclusive: Date,
): { left: number; width: number } | null {
  const ms = monthStart.getTime()
  const me = monthEndExclusive.getTime()
  const ss = shiftStart.getTime()
  const se = shiftEnd.getTime()
  if (se <= ms || ss >= me) return null
  const duration = me - ms
  const left = (Math.max(ss, ms) - ms) / duration
  const right = (Math.min(se, me) - ms) / duration
  const width = right - left
  if (width <= 0) return null
  return { left: left * 100, width: Math.max(width * 100, 0.3) }
}

// ── OnCallCard (one per schedule) ─────────────────────────────────────────────

interface OnCallCardProps {
  tenant: string
  schedule: Schedule
  oncall: OnCallNow | null | undefined
  isLoading: boolean
  userMap: Record<string, string>
}

function OnCallCard({ schedule, oncall, isLoading, userMap }: OnCallCardProps) {
  // Prefer the Keycloak member name; fall back to the oncall-provided username
  // only when it's not a raw uid (the backend returns the uid when it can't
  // resolve a profile).
  const label = oncall
    ? (userMap[oncall.user_id] ??
      (oncall.username !== oncall.user_id ? oncall.username : oncall.user_id.slice(0, 8)))
    : ''
  return (
    <div className="flex min-w-48 flex-col gap-1.5 rounded-lg border p-3">
      <p className="text-xs font-medium text-muted-foreground">{schedule.name}</p>
      {isLoading ? (
        <p className="text-sm text-muted-foreground">...</p>
      ) : !oncall ? (
        <p className="text-sm font-medium text-amber-600 dark:text-amber-400">
          Дежурный не назначен
        </p>
      ) : (
        <div className="flex items-center gap-1.5">
          <User size={13} className="shrink-0 text-muted-foreground" />
          <span className="text-sm font-medium">{label}</span>
        </div>
      )}
    </div>
  )
}

// ── GanttGrid (desktop, task 6.2 + 6.6) ─────────────────────────────────────

interface GanttGridProps {
  schedules: Schedule[]
  windows: Record<string, ShiftWindow[]>
  monthStart: Date
  userMap: Record<string, string>
  onEditSchedule: (schedule: Schedule) => void
  onDeleteSchedule: (schedule: Schedule) => void
}

function GanttGrid({
  schedules,
  windows,
  monthStart,
  userMap,
  onEditSchedule,
  onDeleteSchedule,
}: GanttGridProps) {
  const daysCount = getDaysInMonth(monthStart)
  const days = Array.from({ length: daysCount }, (_, i) => addDays(monthStart, i))
  // startOfMonth(addMonths) gives exactly midnight of the first day of next month
  // (July 1 00:00), so the month spans exactly N whole days and bar percentages
  // align with the day columns (each column = 1/N of the container width).
  // addDays(endOfMonth, 1) would give July 1 23:59:59.999 → ~31-day span → bars
  // drift left by ≈1 column by the end of the month and June 30 appears empty.
  const monthEndExclusive = startOfMonth(addMonths(monthStart, 1))

  if (schedules.length === 0) {
    return (
      <p className="py-12 text-center text-sm text-muted-foreground">Расписаний пока нет.</p>
    )
  }

  return (
    <div className="overflow-x-auto rounded-lg border">
      <div className="min-w-[640px]">
        {/* Day header */}
        <div className="flex border-b bg-muted/30">
          <div className="w-44 shrink-0 border-r px-3 py-2 text-xs font-medium text-muted-foreground">
            Расписание
          </div>
          <div className="flex flex-1">
            {days.map((day, i) => (
              <div
                key={i}
                className={cn(
                  'flex-1 border-l py-1 text-center',
                  isToday(day) && 'bg-primary/10',
                )}
              >
                <div className="text-xs font-medium leading-none">{format(day, 'd')}</div>
                <div className="text-[10px] text-muted-foreground/70">
                  {format(day, 'EEEEE')}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Schedule rows */}
        {schedules.map((schedule) => {
          const shifts = windows[schedule.id] ?? []
          return (
            <div key={schedule.id} className="flex border-b last:border-0 hover:bg-muted/10">
              {/* Row header */}
              <div className="flex w-44 shrink-0 items-center gap-1 border-r px-3 py-2">
                <span className="flex-1 truncate text-sm font-medium">{schedule.name}</span>
                <button
                  onClick={() => onEditSchedule(schedule)}
                  title="Редактировать расписание"
                  className="shrink-0 rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <Pencil size={13} />
                </button>
                <button
                  onClick={() => onDeleteSchedule(schedule)}
                  title="Удалить расписание"
                  className="shrink-0 rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-destructive"
                >
                  <Trash2 size={13} />
                </button>
              </div>

              {/* Track area */}
              <div className="relative flex-1 py-2">
                {/* Vertical day gridlines + today highlight */}
                <div className="pointer-events-none absolute inset-0 flex">
                  {days.map((day, i) => (
                    <div
                      key={i}
                      className={cn('flex-1 border-l border-border/40', isToday(day) && 'bg-primary/5')}
                    />
                  ))}
                </div>

                {/* Shift bars (task 6.6: overrides get dashed border) */}
                <div className="relative h-7">
                  {shifts.map((shift, i) => {
                    const pos = barPosition(
                      new Date(shift.start_at),
                      new Date(shift.end_at),
                      monthStart,
                      monthEndExclusive,
                    )
                    if (!pos) return null
                    const label = userMap[shift.user_id] ?? shift.user_id.slice(0, 8)
                    return (
                      <div
                        key={i}
                        style={{ left: `${pos.left}%`, width: `${pos.width}%` }}
                        className={cn(
                          'absolute inset-y-0.5 flex items-center overflow-hidden rounded-sm px-1',
                          userBarColor(shift.user_id),
                          shift.is_override
                            ? 'border-2 border-dashed border-white/60 opacity-100'
                            : 'opacity-80',
                        )}
                        title={`${label}${shift.is_override ? ' (замена)' : ''} · ${format(new Date(shift.start_at), 'dd.MM HH:mm')} — ${format(new Date(shift.end_at), 'dd.MM HH:mm')}`}
                      >
                        <span className="truncate text-[10px] font-medium leading-none text-white">
                          {label}
                        </span>
                      </div>
                    )
                  })}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ── UpcomingShiftsList (mobile fallback, task 6.5) ────────────────────────────

interface UpcomingShiftsListProps {
  schedules: Schedule[]
  windows: Record<string, ShiftWindow[]>
  userMap: Record<string, string>
}

function UpcomingShiftsList({ schedules, windows, userMap }: UpcomingShiftsListProps) {
  const now = startOfDay(new Date())
  const cutoff = addDays(now, 7)

  const upcoming = schedules
    .flatMap((s) =>
      (windows[s.id] ?? [])
        .filter((w) => new Date(w.end_at) > now && new Date(w.start_at) < cutoff)
        .map((w) => ({ ...w, scheduleName: s.name })),
    )
    .sort((a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime())

  if (upcoming.length === 0) {
    return (
      <p className="py-10 text-center text-sm text-muted-foreground">
        Нет смен на ближайшие 7 дней
      </p>
    )
  }

  return (
    <div className="space-y-2">
      {upcoming.map((shift, i) => {
        const label = userMap[shift.user_id] ?? shift.user_id.slice(0, 8)
        return (
          <div key={i} className="flex items-start gap-3 rounded-lg border p-3">
            <span className={cn('mt-0.5 size-2.5 shrink-0 rounded-full', userBarColor(shift.user_id))} />
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <span className="text-sm font-medium">{label}</span>
                <span className="text-xs text-muted-foreground">{shift.scheduleName}</span>
                {shift.is_override && (
                  <span className="rounded bg-amber-100 px-1 text-xs text-amber-800 dark:bg-amber-900/30 dark:text-amber-400">
                    замена
                  </span>
                )}
              </div>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {format(new Date(shift.start_at), 'dd.MM HH:mm')} —{' '}
                {format(new Date(shift.end_at), 'dd.MM HH:mm')}
              </p>
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── SchedulesPage ─────────────────────────────────────────────────────────────

export function SchedulesPage() {
  const { tenant } = useParams<{ tenant: string }>()
  const t = tenant!

  // Month state for Gantt navigation (task 6.4)
  const [currentMonth, setCurrentMonth] = useState(() => startOfMonth(new Date()))
  const [scheduleModal, setScheduleModal] = useState<{ schedule?: Schedule } | null>(null)
  const [deleteModal, setDeleteModal] = useState<{ schedule: Schedule } | null>(null)

  const { data: rawSchedules } = useSchedules(t)
  const { data: rawMembers } = useMembers(t)

  const schedules = Array.isArray(rawSchedules) ? rawSchedules : []
  const members = Array.isArray(rawMembers) ? rawMembers : []

  const monthStart = currentMonth
  const monthNextStart = startOfMonth(addMonths(currentMonth, 1))
  const from = monthStart.toISOString()
  const to = monthNextStart.toISOString()

  // Parallel queries: oncall-now per schedule (task 6.1, refetch 60s)
  // No ?at param → backend defaults to time.Now()
  const oncallResults = useQueries({
    queries: schedules.map((s) => ({
      queryKey: scheduleKeys(t).oncall(s.id),
      queryFn: async () => {
        const { data } = await apiClient.get<OnCallNow | null>(
          `/schedules/v1/${t}/schedules/${s.id}/oncall`,
        )
        return data
      },
      refetchInterval: 60_000,
    })),
  })

  // Parallel queries: shift windows for the displayed month (task 6.2, 6.3)
  // /shifts returns []domain.Shift (array); /oncall returns a single OncallResult
  const windowResults = useQueries({
    queries: schedules.map((s) => ({
      queryKey: scheduleKeys(t).window(s.id, from, to),
      queryFn: async () => {
        const { data } = await apiClient.get<ShiftWindow[]>(
          `/schedules/v1/${t}/schedules/${s.id}/shifts`,
          { params: { from, to } },
        )
        return data
      },
    })),
  })

  // Build lookup maps
  const windows: Record<string, ShiftWindow[]> = {}
  schedules.forEach((s, i) => {
    const d = windowResults[i]?.data
    windows[s.id] = Array.isArray(d) ? d : []
  })

  // Separate [today, today+7d] window for the mobile "Ближайшие 7 дней" list,
  // independent of the displayed Gantt month — otherwise shifts spilling into
  // the next month are lost at the month boundary (task 13).
  const upcomingFrom = startOfDay(new Date()).toISOString()
  const upcomingTo = addDays(startOfDay(new Date()), 7).toISOString()
  const upcomingResults = useQueries({
    queries: schedules.map((s) => ({
      queryKey: scheduleKeys(t).window(s.id, upcomingFrom, upcomingTo),
      queryFn: async () => {
        const { data } = await apiClient.get<ShiftWindow[]>(
          `/schedules/v1/${t}/schedules/${s.id}/shifts`,
          { params: { from: upcomingFrom, to: upcomingTo } },
        )
        return data
      },
    })),
  })
  const upcomingWindows: Record<string, ShiftWindow[]> = {}
  schedules.forEach((s, i) => {
    const d = upcomingResults[i]?.data
    upcomingWindows[s.id] = Array.isArray(d) ? d : []
  })

  // user_id → display label for Gantt bars.
  // Primary source: Keycloak members for this tenant (useMembers) — authoritative.
  // Supplement: oncall-now results fill gaps for users not in the members list, but
  // must NOT override a member name and must not be a raw uid (the oncall backend
  // returns the uid as username when it can't resolve a profile) — otherwise the
  // currently on-call engineer shows up as a uid everywhere.
  const userMap: Record<string, string> = {}
  members.forEach((m) => { userMap[m.user_id] = m.preferred_username })
  oncallResults.forEach((r) => {
    const d = r.data
    if (d?.user_id && d.username && d.username !== d.user_id && !userMap[d.user_id]) {
      userMap[d.user_id] = d.username
    }
  })

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      {/* OnCall Now Widget (task 6.1) */}
      <section>
        <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Кто дежурит сейчас
        </h2>
        {schedules.length === 0 ? (
          <div className="flex min-w-48 flex-col gap-1.5 rounded-lg border p-3">
            <p className="text-sm text-muted-foreground">Расписаний нет</p>
          </div>
        ) : (
          <div className="flex flex-wrap gap-3">
            {schedules.map((s, i) => (
              <OnCallCard
                key={s.id}
                tenant={t}
                schedule={s}
                oncall={oncallResults[i]?.data}
                isLoading={oncallResults[i]?.isLoading ?? true}
                userMap={userMap}
              />
            ))}
          </div>
        )}
      </section>

      {/* Month navigation + Create override (task 6.4) */}
      <div className="flex items-center gap-2">
        <button
          onClick={() => setCurrentMonth((m) => addMonths(m, -1))}
          className="rounded p-1 hover:bg-muted"
        >
          <ChevronLeft size={16} />
        </button>
        <span className="min-w-28 text-center text-sm font-medium">
          {format(currentMonth, 'LLLL yyyy', { locale: ru })}
        </span>
        <button
          onClick={() => setCurrentMonth((m) => addMonths(m, 1))}
          className="rounded p-1 hover:bg-muted"
        >
          <ChevronRight size={16} />
        </button>
        <Button
          size="sm"
          className="ml-auto"
          onClick={() => setScheduleModal({})}
        >
          <Plus size={14} className="mr-1" />
          Создать расписание
        </Button>
      </div>

      {/* Gantt grid — desktop only (task 6.2, 6.3) */}
      <div className="hidden sm:block">
        <GanttGrid
          schedules={schedules}
          windows={windows}
          monthStart={monthStart}
          userMap={userMap}
          onEditSchedule={(schedule) => setScheduleModal({ schedule })}
          onDeleteSchedule={(schedule) => setDeleteModal({ schedule })}
        />
      </div>

      {/* Mobile fallback — upcoming 7 days (task 6.5) */}
      <div className="block sm:hidden">
        <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Ближайшие 7 дней
        </h2>
        <UpcomingShiftsList schedules={schedules} windows={upcomingWindows} userMap={userMap} />
      </div>

      {/* ScheduleFormModal — create / edit */}
      {scheduleModal && (
        <ScheduleFormModal
          tenant={t}
          members={members}
          schedule={scheduleModal.schedule}
          onClose={() => setScheduleModal(null)}
        />
      )}

      {/* DeleteScheduleModal — confirm deletion */}
      {deleteModal && (
        <DeleteScheduleModal
          tenant={t}
          schedule={deleteModal.schedule}
          onClose={() => setDeleteModal(null)}
        />
      )}
    </div>
  )
}

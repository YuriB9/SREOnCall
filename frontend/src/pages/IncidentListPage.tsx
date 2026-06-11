import { format } from 'date-fns'
import { useEffect, useRef } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'

import {
  type IncidentFilters,
  useAcknowledgeIncident,
  useIncidents,
  useResolveIncident,
} from '@/api/incidents'
import type { IncidentSeverity, IncidentStatus } from '@/api/types'
import { useAudioEnabled } from '@/hooks/useAudioEnabled'
import { useAudioNotification } from '@/hooks/useAudioNotification'
import { useKeyMap } from '@/hooks/useKeyMap'
import { showToast } from '@/lib/toast'
import { cn } from '@/lib/utils'

import { IncidentDetailPanel } from './IncidentDetailPanel'

const STATUSES: IncidentStatus[] = ['open', 'acknowledged', 'resolved']

const STATUS_LABEL: Record<IncidentStatus, string> = {
  open: 'Открытые',
  acknowledged: 'Подтверждённые',
  resolved: 'Закрытые',
}

const STATUS_CLASS: Record<IncidentStatus, string> = {
  open: 'bg-destructive/10 text-destructive ring-destructive/30',
  acknowledged: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400 ring-yellow-500/30',
  resolved: 'bg-green-500/10 text-green-700 dark:text-green-400 ring-green-500/30',
}

const SEVERITY_LABEL: Record<IncidentSeverity, string> = {
  critical: 'Критический',
  high: 'Высокий',
  warning: 'Предупреждение',
  info: 'Инфо',
}

const SEVERITY_CLASS: Record<IncidentSeverity, string> = {
  critical: 'bg-destructive/10 text-destructive',
  high: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
  warning: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
  info: 'bg-secondary text-secondary-foreground',
}

const STATUS_CHIP: Record<IncidentStatus, string> = {
  open: 'bg-destructive/10 text-destructive',
  acknowledged: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
  resolved: 'bg-green-500/10 text-green-700 dark:text-green-400',
}

const STATUS_TEXT: Record<IncidentStatus, string> = {
  open: 'Открыт',
  acknowledged: 'Подтверждён',
  resolved: 'Закрыт',
}

export function IncidentListPage() {
  const { tenant } = useParams<{ tenant: string }>()
  const t = tenant!
  const [searchParams, setSearchParams] = useSearchParams()

  const statusFilter = searchParams.getAll('status') as IncidentStatus[]
  const severityFilter = (searchParams.get('severity') ?? '') as IncidentSeverity | ''
  const selectedId = searchParams.get('incident') ?? ''

  const filters: IncidentFilters = {
    ...(statusFilter.length ? { status: statusFilter.join(',') } : {}),
    ...(severityFilter ? { severity: severityFilter } : {}),
  }

  const { data, isLoading } = useIncidents(t, filters)
  const incidents = data?.incidents ?? []

  const [audioEnabled] = useAudioEnabled()
  const playBeep = useAudioNotification()
  const prevOpenIdsRef = useRef<Set<string> | null>(null)

  useEffect(() => {
    if (!data?.incidents) return
    const newOpenIds = new Set(
      data.incidents.filter((i) => i.status === 'open').map((i) => i.id),
    )
    const prev = prevOpenIdsRef.current
    if (prev !== null) {
      const hasNew = [...newOpenIds].some((id) => !prev.has(id))
      if (hasNew && audioEnabled) playBeep()
    }
    prevOpenIdsRef.current = newOpenIds
  }, [data, audioEnabled, playBeep])

  const selectedIndex = incidents.findIndex((i) => i.id === selectedId)
  const selectedIncident = incidents[selectedIndex]

  function setParam(key: string, value: string | null) {
    setSearchParams((p) => {
      const params = new URLSearchParams(p)
      if (value) params.set(key, value)
      else params.delete(key)
      return params
    })
  }

  function toggleStatus(s: IncidentStatus) {
    setSearchParams((p) => {
      const params = new URLSearchParams(p)
      params.delete('status')
      params.delete('page')
      const current = p.getAll('status') as IncidentStatus[]
      const next = current.includes(s) ? current.filter((x) => x !== s) : [...current, s]
      next.forEach((v) => params.append('status', v))
      return params
    })
  }

  function selectIncident(id: string | null) {
    setParam('incident', id)
  }

  function closePanel() {
    setParam('incident', null)
  }

  const ack = useAcknowledgeIncident(t)
  const resolve = useResolveIncident(t)

  function navDown() {
    if (!incidents.length) return
    const next = selectedIndex < incidents.length - 1 ? selectedIndex + 1 : 0
    selectIncident(incidents[next].id)
  }

  function navUp() {
    if (!incidents.length) return
    const prev = selectedIndex > 0 ? selectedIndex - 1 : incidents.length - 1
    selectIncident(incidents[prev].id)
  }

  useKeyMap({
    a: () => {
      if (selectedId) {
        ack.mutate(selectedId, { onError: () => showToast('Не удалось подтвердить инцидент') })
      }
    },
    A: () => {
      if (selectedId) {
        ack.mutate(selectedId, { onError: () => showToast('Не удалось подтвердить инцидент') })
      }
    },
    r: () => {
      if (selectedId && selectedIncident?.status !== 'resolved') {
        resolve.mutate(selectedId, { onError: () => showToast('Не удалось закрыть инцидент') })
      }
    },
    R: () => {
      if (selectedId && selectedIncident?.status !== 'resolved') {
        resolve.mutate(selectedId, { onError: () => showToast('Не удалось закрыть инцидент') })
      }
    },
    j: navDown,
    J: navDown,
    k: navUp,
    K: navUp,
    Escape: closePanel,
  })

  return (
    <div className="flex h-full flex-col">
      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-2 border-b p-3">
        {/* Status toggles */}
        <div className="flex gap-1">
          {STATUSES.map((s) => (
            <button
              key={s}
              onClick={() => toggleStatus(s)}
              className={cn(
                'rounded-full px-3 py-1 text-xs font-medium transition-colors',
                statusFilter.includes(s)
                  ? STATUS_CLASS[s] + ' ring-1'
                  : 'bg-muted text-muted-foreground hover:bg-muted/80',
              )}
            >
              {STATUS_LABEL[s]}
            </button>
          ))}
        </div>

        {/* Severity select */}
        <select
          value={severityFilter}
          onChange={(e) => {
            setSearchParams((p) => {
              const params = new URLSearchParams(p)
              if (e.target.value) params.set('severity', e.target.value)
              else params.delete('severity')
              params.delete('page')
              return params
            })
          }}
          className="h-7 rounded-md border bg-background px-2 text-xs outline-none focus:ring-1 focus:ring-ring"
        >
          <option value="">Все критичности</option>
          <option value="critical">Критический</option>
          <option value="high">Высокий</option>
          <option value="warning">Предупреждение</option>
          <option value="info">Инфо</option>
        </select>

        <span className="ml-auto text-xs text-muted-foreground">
          {isLoading ? 'Обновление...' : `${incidents.length} инцидентов`}
        </span>
      </div>

      {/* Split view */}
      <div className="flex flex-1 overflow-hidden">
        {/* Table */}
        <div className="flex flex-1 flex-col overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-background">
              <tr className="border-b text-left">
                <th className="p-2 pl-3 text-xs font-medium text-muted-foreground">
                  Критичность
                </th>
                <th className="p-2 text-xs font-medium text-muted-foreground">Название</th>
                <th className="p-2 text-xs font-medium text-muted-foreground">Статус</th>
                <th className="p-2 text-xs font-medium text-muted-foreground">Alertname</th>
                <th className="p-2 text-xs font-medium text-muted-foreground">Создан</th>
                <th className="p-2 pr-3 text-xs font-medium text-muted-foreground">
                  Подтверждён
                </th>
              </tr>
            </thead>
            <tbody>
              {incidents.map((inc) => (
                <tr
                  key={inc.id}
                  onClick={() => selectIncident(inc.id === selectedId ? null : inc.id)}
                  className={cn(
                    'cursor-pointer border-b transition-colors hover:bg-muted/50',
                    inc.id === selectedId && 'bg-accent',
                  )}
                >
                  <td className="p-2 pl-3">
                    <span
                      className={cn(
                        'inline-flex h-5 items-center rounded-full px-2 text-xs font-medium',
                        SEVERITY_CLASS[inc.severity],
                      )}
                    >
                      {SEVERITY_LABEL[inc.severity]}
                    </span>
                  </td>
                  <td className="max-w-xs p-2">
                    <span className="block truncate font-medium">{inc.title}</span>
                  </td>
                  <td className="p-2">
                    <span
                      className={cn(
                        'inline-flex h-5 items-center rounded-full px-2 text-xs font-medium',
                        STATUS_CHIP[inc.status],
                      )}
                    >
                      {STATUS_TEXT[inc.status]}
                    </span>
                  </td>
                  <td className="p-2 text-xs text-muted-foreground">
                    {inc.labels?.alertname ?? '—'}
                  </td>
                  <td className="p-2 text-xs text-muted-foreground">
                    {format(new Date(inc.created_at), 'dd.MM HH:mm')}
                  </td>
                  <td className="p-2 pr-3 text-xs text-muted-foreground">
                    {inc.acknowledged_by ?? '—'}
                  </td>
                </tr>
              ))}

              {!isLoading && incidents.length === 0 && (
                <tr>
                  <td colSpan={6} className="p-10 text-center text-muted-foreground">
                    Инцидентов нет
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          {data?.next_cursor && (
            <div className="mt-auto border-t p-3 text-center">
              <span className="text-xs text-muted-foreground">
                Показано {incidents.length} инцидентов
              </span>
            </div>
          )}
        </div>

        {/* Detail panel */}
        {selectedId && (
          <IncidentDetailPanel
            key={selectedId}
            tenant={t}
            incidentId={selectedId}
            onClose={closePanel}
          />
        )}
      </div>
    </div>
  )
}

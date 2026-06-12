import { format } from 'date-fns'
import type { LucideIcon } from 'lucide-react'
import {
  ArrowUpCircle,
  Ban,
  BellRing,
  Check,
  CheckCircle2,
  Clock,
  MessageSquare,
  Send,
  Tag,
  X,
} from 'lucide-react'
import { useState } from 'react'

import { useEscalationHistory } from '@/api/escalations'
import {
  useAcknowledgeIncident,
  useIncident,
  useIncidentAlerts,
  useIncidentComments,
  useIncidentHistory,
  usePostComment,
  useResolveIncident,
} from '@/api/incidents'
import type {
  EscalationHistoryEventType,
  HistoryEventKind,
} from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { showToast } from '@/lib/toast'
import { cn } from '@/lib/utils'

interface Props {
  tenant: string
  incidentId: string
  onClose: () => void
}

const HISTORY_ICON: Record<HistoryEventKind, LucideIcon> = {
  status_change: Clock,
  label_change: Tag,
  comment_added: MessageSquare,
}

const HISTORY_LABEL: Record<HistoryEventKind, string> = {
  status_change: 'Статус изменён',
  label_change: 'Лейбл изменён',
  comment_added: 'Комментарий',
}

const ESCALATION_ICON: Record<EscalationHistoryEventType, LucideIcon> = {
  triggered: BellRing,
  tier_advanced: ArrowUpCircle,
  acknowledged: Check,
  resolved: CheckCircle2,
  exhausted: Ban,
}

const ESCALATION_LABEL: Record<EscalationHistoryEventType, string> = {
  triggered: 'Эскалация запущена',
  tier_advanced: 'Переход на уровень',
  acknowledged: 'Эскалация подтверждена',
  resolved: 'Эскалация разрешена',
  exhausted: 'Уровни исчерпаны',
}

// A single entry in the merged history timeline, normalized from either the
// incident journal or the escalation history so both can be sorted and rendered
// together.
interface TimelineItem {
  key: string
  time: number
  occurredAt: string
  icon: LucideIcon
  label: string
  lines: string[]
}

export function IncidentDetailPanel({ tenant, incidentId, onClose }: Props) {
  const { data: incident, isLoading } = useIncident(tenant, incidentId)
  const { data: alertsData } = useIncidentAlerts(tenant, incidentId)
  const { data: historyData } = useIncidentHistory(tenant, incidentId)
  const { data: commentsData } = useIncidentComments(tenant, incidentId)
  const { data: escalationHistoryData } = useEscalationHistory(tenant, incidentId)

  const alerts = Array.isArray(alertsData) ? alertsData : []
  const history = Array.isArray(historyData) ? historyData : []
  const comments = Array.isArray(commentsData) ? commentsData : []
  const escalationHistory = Array.isArray(escalationHistoryData)
    ? escalationHistoryData
    : []

  // Normalize both the incident journal and the escalation history into a single
  // chronologically sorted timeline. Degrades gracefully: when the escalation
  // service is unavailable the hook returns [], so only incident events show.
  const timeline: TimelineItem[] = [
    ...history.map((entry): TimelineItem => {
      const lines: string[] = []
      if (entry.author) lines.push(entry.author)
      if (
        entry.kind !== 'comment_added' &&
        entry.old_value !== undefined &&
        entry.new_value !== undefined
      ) {
        lines.push(`${entry.old_value || '—'} → ${entry.new_value}`)
      }
      return {
        key: `incident:${entry.id}`,
        time: Date.parse(entry.occurred_at),
        occurredAt: entry.occurred_at,
        icon: HISTORY_ICON[entry.kind],
        label: HISTORY_LABEL[entry.kind],
        lines,
      }
    }),
    ...escalationHistory.map((entry): TimelineItem => {
      const lines: string[] = []
      if (entry.event_type === 'triggered' || entry.event_type === 'tier_advanced') {
        const parts: string[] = []
        if (entry.tier !== undefined) parts.push(`Уровень ${entry.tier}`)
        if (entry.oncall_username) parts.push(entry.oncall_username)
        if (parts.length > 0) lines.push(parts.join(' · '))
      } else if (entry.oncall_username) {
        lines.push(entry.oncall_username)
      }
      return {
        key: `escalation:${entry.id}`,
        time: entry.created_at ? Date.parse(entry.created_at) : 0,
        occurredAt: entry.created_at ?? '',
        icon: ESCALATION_ICON[entry.event_type],
        label: ESCALATION_LABEL[entry.event_type],
        lines,
      }
    }),
  ].sort((a, b) => a.time - b.time)

  const ack = useAcknowledgeIncident(tenant)
  const resolve = useResolveIncident(tenant)
  const postComment = usePostComment(tenant, incidentId)

  const [commentText, setCommentText] = useState('')

  if (isLoading || !incident) {
    return (
      <div className="flex w-96 flex-shrink-0 items-center justify-center border-l p-4">
        <span className="text-sm text-muted-foreground">Загрузка...</span>
      </div>
    )
  }

  const isResolved = incident.status === 'resolved'
  const isAcknowledged = incident.status === 'acknowledged'

  function handleSubmitComment() {
    const text = commentText.trim()
    if (!text) return
    postComment.mutate(text, {
      onSuccess: () => setCommentText(''),
      onError: () => showToast('Не удалось отправить комментарий'),
    })
  }

  return (
    <div className="flex w-96 flex-shrink-0 flex-col overflow-hidden border-l">
      {/* Header */}
      <div className="flex items-start gap-2 border-b p-3">
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{incident.title}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {incident.labels?.alertname ?? incident.tenant_id}
          </p>
        </div>
        <button
          onClick={onClose}
          aria-label="Закрыть"
          className="shrink-0 rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
        >
          <X size={14} />
        </button>
      </div>

      {/* Action buttons */}
      <div className="flex gap-2 border-b p-3">
        <Button
          size="sm"
          variant="outline"
          disabled={isAcknowledged || isResolved || ack.isPending}
          onClick={() =>
            ack.mutate(incidentId, {
              onError: () => showToast('Не удалось подтвердить инцидент'),
            })
          }
        >
          Подтвердить
        </Button>
        <Button
          size="sm"
          variant="destructive"
          disabled={isResolved || resolve.isPending}
          onClick={() =>
            resolve.mutate(incidentId, {
              onError: () => showToast('Не удалось закрыть инцидент'),
            })
          }
        >
          Закрыть
        </Button>
      </div>

      {/* Tabs */}
      <Tabs defaultValue="alerts" className="flex min-h-0 flex-1 flex-col gap-0">
        <TabsList className="mx-3 mt-2 shrink-0 w-fit">
          <TabsTrigger value="alerts">Алерты ({alerts.length})</TabsTrigger>
          <TabsTrigger value="history">История ({timeline.length})</TabsTrigger>
          <TabsTrigger value="comments">Комменты ({comments.length})</TabsTrigger>
        </TabsList>

        {/* Alerts */}
        <TabsContent value="alerts" className="mt-2 min-h-0 flex-1 overflow-auto px-3 pb-3">
          {alerts.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">Нет алертов</p>
          ) : (
            <ul className="space-y-2">
              {alerts.map((alert) => (
                <li key={alert.id} className="rounded-md border p-2">
                  <div className="mb-1.5 flex items-center gap-1.5">
                    <span
                      className={cn(
                        'inline-block size-2 rounded-full',
                        alert.status === 'firing' ? 'bg-destructive' : 'bg-green-500',
                      )}
                    />
                    <span className="text-xs font-medium">
                      {alert.status === 'firing' ? 'Срабатывает' : 'Разрешён'}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1">
                    <Badge variant="secondary" className="text-xs">
                      source: {alert.source}
                    </Badge>
                    <Badge
                      variant="secondary"
                      className="text-xs font-mono"
                      title={alert.fingerprint}
                    >
                      fingerprint: {alert.fingerprint.slice(0, 8)}
                    </Badge>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </TabsContent>

        {/* History */}
        <TabsContent value="history" className="mt-2 min-h-0 flex-1 overflow-auto px-3 pb-3">
          {timeline.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">История пуста</p>
          ) : (
            <ol>
              {timeline.map((item, i) => {
                const Icon = item.icon
                return (
                  <li key={item.key} className="flex gap-3">
                    <div className="flex flex-col items-center">
                      <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted">
                        <Icon size={12} />
                      </div>
                      {i < timeline.length - 1 && (
                        <div className="w-px flex-1 bg-border" />
                      )}
                    </div>
                    <div className="pb-4 pt-0.5">
                      <p className="text-xs font-medium">{item.label}</p>
                      {item.lines.map((line, j) => (
                        <p key={j} className="text-xs text-muted-foreground">
                          {line}
                        </p>
                      ))}
                      {item.occurredAt && (
                        <p className="mt-0.5 text-xs text-muted-foreground/60">
                          {format(new Date(item.occurredAt), 'dd.MM HH:mm')}
                        </p>
                      )}
                    </div>
                  </li>
                )
              })}
            </ol>
          )}
        </TabsContent>

        {/* Comments */}
        <TabsContent
          value="comments"
          className="mt-2 flex min-h-0 flex-1 flex-col overflow-hidden"
        >
          <div className="min-h-0 flex-1 overflow-auto px-3 pb-2">
            {comments.length === 0 ? (
              <p className="py-6 text-center text-xs text-muted-foreground">Нет комментариев</p>
            ) : (
              <div className="space-y-2">
                {comments.map((c) => (
                  <div key={c.id} className="rounded-md bg-muted/50 p-2">
                    <div className="mb-0.5 flex items-center justify-between">
                      <span className="text-xs font-medium">{c.author_id}</span>
                      <span className="text-xs text-muted-foreground">
                        {format(new Date(c.created_at), 'dd.MM HH:mm')}
                      </span>
                    </div>
                    <p className="text-xs">{c.body}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
          {isResolved ? (
            <p className="shrink-0 border-t p-3 text-center text-xs text-muted-foreground">
              Инцидент закрыт — комментарии недоступны
            </p>
          ) : (
            <div className="flex shrink-0 gap-2 border-t p-3">
              <textarea
                value={commentText}
                onChange={(e) => setCommentText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) handleSubmitComment()
                }}
                placeholder="Комментарий... (Ctrl+Enter)"
                rows={2}
                className="flex-1 resize-none rounded-md border bg-transparent p-2 text-xs outline-none focus:ring-1 focus:ring-ring"
              />
              <Button
                size="icon"
                variant="outline"
                disabled={!commentText.trim() || postComment.isPending}
                onClick={handleSubmitComment}
                aria-label="Отправить"
              >
                <Send size={14} />
              </Button>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}

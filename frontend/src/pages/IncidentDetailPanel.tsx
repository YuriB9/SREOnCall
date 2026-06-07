import { format } from 'date-fns'
import type { LucideIcon } from 'lucide-react'
import {
  Clock,
  MessageSquare,
  Send,
  Tag,
  X,
} from 'lucide-react'
import { useState } from 'react'

import {
  useAcknowledgeIncident,
  useIncident,
  useIncidentAlerts,
  useIncidentComments,
  useIncidentHistory,
  usePostComment,
  useResolveIncident,
} from '@/api/incidents'
import type { HistoryEventKind } from '@/api/types'
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

export function IncidentDetailPanel({ tenant, incidentId, onClose }: Props) {
  const { data: incident, isLoading } = useIncident(tenant, incidentId)
  const { data: alertsData } = useIncidentAlerts(tenant, incidentId)
  const { data: historyData } = useIncidentHistory(tenant, incidentId)
  const { data: commentsData } = useIncidentComments(tenant, incidentId)

  const alerts = Array.isArray(alertsData) ? alertsData : []
  const history = Array.isArray(historyData) ? historyData : []
  const comments = Array.isArray(commentsData) ? commentsData : []

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
          <TabsTrigger value="history">История</TabsTrigger>
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
                    <Badge variant="secondary" className="text-xs font-mono">
                      {alert.fingerprint}
                    </Badge>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </TabsContent>

        {/* History */}
        <TabsContent value="history" className="mt-2 min-h-0 flex-1 overflow-auto px-3 pb-3">
          {history.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">История пуста</p>
          ) : (
            <ol>
              {history.map((entry, i) => {
                const Icon = HISTORY_ICON[entry.kind]
                return (
                  <li key={entry.id} className="flex gap-3">
                    <div className="flex flex-col items-center">
                      <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted">
                        <Icon size={12} />
                      </div>
                      {i < history.length - 1 && (
                        <div className="w-px flex-1 bg-border" />
                      )}
                    </div>
                    <div className="pb-4 pt-0.5">
                      <p className="text-xs font-medium">{HISTORY_LABEL[entry.kind]}</p>
                      {entry.author && (
                        <p className="text-xs text-muted-foreground">{entry.author}</p>
                      )}
                      {entry.old_value !== undefined && entry.new_value !== undefined && (
                        <p className="text-xs text-muted-foreground">
                          {entry.old_value || '—'} → {entry.new_value}
                        </p>
                      )}
                      <p className="mt-0.5 text-xs text-muted-foreground/60">
                        {format(new Date(entry.occurred_at), 'dd.MM HH:mm')}
                      </p>
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
        </TabsContent>
      </Tabs>
    </div>
  )
}

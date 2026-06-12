import { Info } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { useNotificationConfig, useSaveNotificationConfig } from '@/api/tenantSettings'
import type { NotificationConfig } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

function isValidUrl(val: string) {
  if (!val) return true
  try {
    new URL(val)
    return true
  } catch {
    return false
  }
}

function isValidEmail(val: string) {
  if (!val) return true
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val)
}

// ── NotificationConfigSection ─────────────────────────────────────────────────

export function NotificationConfigSection({ tenant }: { tenant: string }) {
  const { data: serverConfig } = useNotificationConfig(tenant)
  const saveConfig = useSaveNotificationConfig(tenant)

  const [webhookUrl, setWebhookUrl] = useState('')
  const [channel, setChannel] = useState('')
  const [smtpFrom, setSmtpFrom] = useState('')
  const [errors, setErrors] = useState<{ webhookUrl?: string; smtpFrom?: string }>({})
  const initialized = useRef(false)

  // GET returns a masked URL (https://host/***). Never pre-populate webhookUrl with it
  // — saving the masked value back would corrupt the stored URL. Channel and smtp_from
  // are safe to pre-fill since they are not masked.
  const maskedWebhookUrl = serverConfig?.mattermost_webhook_url ?? ''

  useEffect(() => {
    if (serverConfig && !initialized.current) {
      setChannel(serverConfig.mattermost_channel ?? '')
      setSmtpFrom(serverConfig.smtp_from ?? '')
      initialized.current = true
    }
  }, [serverConfig])

  function validate(): boolean {
    const errs: typeof errors = {}
    if (webhookUrl && !isValidUrl(webhookUrl)) errs.webhookUrl = 'Введите корректный URL'
    if (smtpFrom && !isValidEmail(smtpFrom)) errs.smtpFrom = 'Введите корректный email'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!validate()) return
    // Не отправляем mattermost_webhook_url, если поле не заполнялось:
    // пустое значение трактуется сервером как «оставить текущее», а лишнее
    // поле в теле не должно затирать сохранённый вебхук.
    const body: Partial<NotificationConfig> = {
      mattermost_channel: channel,
      smtp_from: smtpFrom,
    }
    if (webhookUrl) body.mattermost_webhook_url = webhookUrl
    saveConfig.mutate(body, {
      onSuccess: () => showToast('Конфигурация сохранена', 'success'),
      onError: () => showToast('Не удалось сохранить конфигурацию'),
    })
  }

  return (
    <section className="rounded-lg border p-5">
      <div className="mb-4 flex items-center gap-2">
        <Info size={16} className="text-muted-foreground" />
        <h2 className="text-sm font-semibold">Конфигурация уведомлений</h2>
      </div>

      <form onSubmit={handleSubmit} className="flex max-w-lg flex-col gap-4">
        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Mattermost Webhook URL</label>
          <input
            type="text"
            value={webhookUrl}
            onChange={(e) => { setWebhookUrl(e.target.value); setErrors((p) => ({ ...p, webhookUrl: undefined })) }}
            placeholder="https://mattermost.example.com/hooks/..."
            className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
          {errors.webhookUrl && <p className="text-xs text-destructive">{errors.webhookUrl}</p>}
          {maskedWebhookUrl && (
            <p className="text-xs text-muted-foreground">
              Текущий URL скрыт: <span className="font-mono">{maskedWebhookUrl}</span>
              {' — '}введите полный URL для изменения, или оставьте поле пустым, чтобы сохранить текущий.
            </p>
          )}
        </div>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Mattermost канал</label>
          <input
            type="text"
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
            placeholder="#oncall-alerts"
            className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">SMTP From (email)</label>
          <input
            type="text"
            value={smtpFrom}
            onChange={(e) => { setSmtpFrom(e.target.value); setErrors((p) => ({ ...p, smtpFrom: undefined })) }}
            placeholder="oncall@example.com"
            className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
          {errors.smtpFrom && <p className="text-xs text-destructive">{errors.smtpFrom}</p>}
        </div>

        <div>
          <Button type="submit" size="sm" disabled={saveConfig.isPending}>
            {saveConfig.isPending ? 'Сохранение...' : 'Сохранить'}
          </Button>
        </div>
      </form>
    </section>
  )
}

// ── NotificationConfigPage ────────────────────────────────────────────────────

export function NotificationConfigPage() {
  const { tenant } = useParams<{ tenant: string }>()

  return (
    <div className="flex h-full flex-col gap-5 overflow-auto p-4">
      <h1 className="text-base font-semibold">Конфигурация уведомлений</h1>
      <NotificationConfigSection tenant={tenant!} />
    </div>
  )
}

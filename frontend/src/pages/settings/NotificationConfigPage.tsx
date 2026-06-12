import { Info } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import { useNotificationConfig, useSaveNotificationConfig } from '@/api/tenantSettings'
import type { NotificationConfig } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

const SUBJECT_PREFIX_MAX = 64

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

const inputClass =
  'w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring'

// ── MattermostSection ─────────────────────────────────────────────────────────

function MattermostSection({ tenant }: { tenant: string }) {
  const { data: serverConfig } = useNotificationConfig(tenant)
  const saveConfig = useSaveNotificationConfig(tenant)

  const [enabled, setEnabled] = useState(true)
  const [webhookUrl, setWebhookUrl] = useState('')
  const [channel, setChannel] = useState('')
  const [error, setError] = useState<string | undefined>()
  const initialized = useRef(false)

  // GET returns a masked URL (https://host/***). Never pre-populate webhookUrl with it
  // — saving the masked value back would corrupt the stored URL. Channel is safe to
  // pre-fill since it is not masked.
  const maskedWebhookUrl = serverConfig?.mattermost_webhook_url ?? ''

  useEffect(() => {
    if (serverConfig && !initialized.current) {
      setEnabled(serverConfig.mattermost_enabled ?? true)
      setChannel(serverConfig.mattermost_channel ?? '')
      initialized.current = true
    }
  }, [serverConfig])

  // Enabled but no webhook (neither stored nor freshly typed) → notifications
  // would silently fail; warn the admin softly without blocking the save.
  const missingWebhook = enabled && !webhookUrl && !maskedWebhookUrl

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (webhookUrl && !isValidUrl(webhookUrl)) {
      setError('Введите корректный URL')
      return
    }
    // Не отправляем mattermost_webhook_url, если поле не заполнялось:
    // пустое значение трактуется сервером как «оставить текущее».
    const body: Partial<NotificationConfig> = {
      mattermost_enabled: enabled,
      mattermost_channel: channel,
    }
    if (webhookUrl) body.mattermost_webhook_url = webhookUrl
    saveConfig.mutate(body, {
      onSuccess: () => showToast('Настройки Mattermost сохранены', 'success'),
      onError: () => showToast('Не удалось сохранить настройки Mattermost'),
    })
  }

  return (
    <section className="rounded-lg border p-5">
      <div className="mb-4 flex items-center gap-2">
        <Info size={16} className="text-muted-foreground" />
        <h2 className="text-sm font-semibold">Mattermost</h2>
      </div>

      <form onSubmit={handleSubmit} className="flex max-w-lg flex-col gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="size-4"
          />
          Mattermost-уведомления включены
        </label>

        {missingWebhook && (
          <p className="text-xs text-amber-600 dark:text-amber-500">
            Канал включён, но Webhook URL не задан — уведомления не будут отправляться, пока вы его не укажете.
          </p>
        )}

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Mattermost Webhook URL</label>
          <input
            type="text"
            value={webhookUrl}
            onChange={(e) => { setWebhookUrl(e.target.value); setError(undefined) }}
            placeholder="https://mattermost.example.com/hooks/..."
            className={inputClass}
          />
          {error && <p className="text-xs text-destructive">{error}</p>}
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
            className={inputClass}
          />
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

// ── EmailSection ──────────────────────────────────────────────────────────────

function EmailSection({ tenant }: { tenant: string }) {
  const { data: serverConfig } = useNotificationConfig(tenant)
  const saveConfig = useSaveNotificationConfig(tenant)

  const [enabled, setEnabled] = useState(true)
  const [smtpFrom, setSmtpFrom] = useState('')
  const [replyTo, setReplyTo] = useState('')
  const [subjectPrefix, setSubjectPrefix] = useState('')
  const [errors, setErrors] = useState<{ smtpFrom?: string; replyTo?: string }>({})
  const initialized = useRef(false)

  useEffect(() => {
    if (serverConfig && !initialized.current) {
      setEnabled(serverConfig.email_enabled ?? true)
      setSmtpFrom(serverConfig.smtp_from ?? '')
      setReplyTo(serverConfig.email_reply_to ?? '')
      setSubjectPrefix(serverConfig.email_subject_prefix ?? '')
      initialized.current = true
    }
  }, [serverConfig])

  function validate(): boolean {
    const errs: typeof errors = {}
    if (smtpFrom && !isValidEmail(smtpFrom)) errs.smtpFrom = 'Введите корректный email'
    if (replyTo && !isValidEmail(replyTo)) errs.replyTo = 'Введите корректный email'
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!validate()) return
    const body: Partial<NotificationConfig> = {
      email_enabled: enabled,
      smtp_from: smtpFrom,
      email_reply_to: replyTo,
      email_subject_prefix: subjectPrefix,
    }
    saveConfig.mutate(body, {
      onSuccess: () => showToast('Настройки Email сохранены', 'success'),
      onError: () => showToast('Не удалось сохранить настройки Email'),
    })
  }

  return (
    <section className="rounded-lg border p-5">
      <div className="mb-4 flex items-center gap-2">
        <Info size={16} className="text-muted-foreground" />
        <h2 className="text-sm font-semibold">Email</h2>
      </div>

      <form onSubmit={handleSubmit} className="flex max-w-lg flex-col gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="size-4"
          />
          Email-уведомления включены
        </label>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">SMTP From (email)</label>
          <input
            type="text"
            value={smtpFrom}
            onChange={(e) => { setSmtpFrom(e.target.value); setErrors((p) => ({ ...p, smtpFrom: undefined })) }}
            placeholder="oncall@example.com"
            className={inputClass}
          />
          {errors.smtpFrom && <p className="text-xs text-destructive">{errors.smtpFrom}</p>}
        </div>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Reply-To (email)</label>
          <input
            type="text"
            value={replyTo}
            onChange={(e) => { setReplyTo(e.target.value); setErrors((p) => ({ ...p, replyTo: undefined })) }}
            placeholder="support@example.com"
            className={inputClass}
          />
          {errors.replyTo && <p className="text-xs text-destructive">{errors.replyTo}</p>}
        </div>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Префикс темы письма</label>
          <input
            type="text"
            value={subjectPrefix}
            maxLength={SUBJECT_PREFIX_MAX}
            onChange={(e) => setSubjectPrefix(e.target.value.slice(0, SUBJECT_PREFIX_MAX))}
            placeholder="[ACME PROD]"
            className={inputClass}
          />
          <p className="text-xs text-muted-foreground">
            До {SUBJECT_PREFIX_MAX} символов. Добавляется в начало темы письма.
          </p>
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
      <MattermostSection tenant={tenant!} />
      <EmailSection tenant={tenant!} />
    </div>
  )
}

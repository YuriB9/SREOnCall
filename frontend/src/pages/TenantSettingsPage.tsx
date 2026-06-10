import { AlertTriangle, Check, ClipboardCopy, Info, KeyRound, Plus, Trash2, Users } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'

import {
  useCreateToken,
  useMembers,
  useNotificationConfig,
  useRevokeToken,
  useSaveNotificationConfig,
  useWebhookTokens,
} from '@/api/tenantSettings'
import type { Member, NotificationConfig, WebhookToken } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

// ── Helpers ───────────────────────────────────────────────────────────────────

const VALID_SOURCES = ['alertmanager', 'grafana'] as const
type ValidSource = (typeof VALID_SOURCES)[number]

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  })
}

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

// ── OneTimeTokenRevealModal ───────────────────────────────────────────────────

interface OneTimeTokenRevealModalProps {
  token: string
  onClose: () => void
}

function OneTimeTokenRevealModal({ token, onClose }: OneTimeTokenRevealModalProps) {
  const [copied, setCopied] = useState(false)
  const [confirmed, setConfirmed] = useState(false)

  function handleCopy() {
    navigator.clipboard.writeText(token).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-md rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-1 flex items-center gap-2">
          <KeyRound size={18} className="text-primary" />
          <h2 className="text-sm font-semibold">Webhook-токен создан</h2>
        </div>
        <p className="mb-4 text-xs text-muted-foreground">
          Это единственный раз, когда вы увидите этот токен. Скопируйте его сейчас.
        </p>

        <div className="relative mb-4 rounded-md border bg-muted">
          <pre className="overflow-x-auto p-3 pr-10 font-mono text-xs break-all">{token}</pre>
          <button
            type="button"
            onClick={handleCopy}
            title="Скопировать"
            className="absolute right-2 top-2 rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            {copied ? <Check size={14} className="text-green-500" /> : <ClipboardCopy size={14} />}
          </button>
        </div>

        <label className="mb-4 flex cursor-pointer items-start gap-2 text-sm">
          <input
            type="checkbox"
            checked={confirmed}
            onChange={(e) => setConfirmed(e.target.checked)}
            className="mt-0.5 accent-primary"
          />
          <span>Я скопировал этот токен</span>
        </label>

        <div className="flex justify-end">
          <Button size="sm" disabled={!confirmed} onClick={onClose}>
            Закрыть
          </Button>
        </div>
      </div>
    </div>
  )
}

// ── GenerateTokenModal ────────────────────────────────────────────────────────

interface GenerateTokenModalProps {
  onClose: () => void
  onSuccess: (token: string) => void
  tenant: string
}

function GenerateTokenModal({ onClose, onSuccess, tenant }: GenerateTokenModalProps) {
  const [source, setSource] = useState<ValidSource>('alertmanager')
  const createToken = useCreateToken(tenant)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    createToken.mutate(source, {
      onSuccess: (data) => {
        onSuccess(data.token)
      },
      onError: () => showToast('Не удалось создать токен'),
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-sm rounded-xl border bg-background p-6 shadow-xl">
        <h2 className="mb-4 text-sm font-semibold">Новый webhook-токен</h2>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Источник</label>
            <select
              value={source}
              onChange={(e) => setSource(e.target.value as ValidSource)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              {VALID_SOURCES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={onClose} disabled={createToken.isPending}>
              Отмена
            </Button>
            <Button type="submit" size="sm" disabled={createToken.isPending}>
              {createToken.isPending ? 'Создание...' : 'Создать'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ── RevokeConfirmDialog ───────────────────────────────────────────────────────

interface RevokeDialogProps {
  token: WebhookToken
  onCancel: () => void
  onConfirm: () => void
  isPending: boolean
}

function RevokeConfirmDialog({ token, onCancel, onConfirm, isPending }: RevokeDialogProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-sm rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-4 flex items-start gap-3">
          <AlertTriangle size={18} className="mt-0.5 shrink-0 text-destructive" />
          <div>
            <h2 className="text-sm font-semibold">Отозвать токен?</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Токен <span className="font-medium">{token.source}</span> будет удалён. Интеграции, использующие его, перестанут работать.
            </p>
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={isPending}>
            Отмена
          </Button>
          <Button variant="destructive" size="sm" onClick={onConfirm} disabled={isPending}>
            {isPending ? 'Отзыв...' : 'Отозвать'}
          </Button>
        </div>
      </div>
    </div>
  )
}

// ── WebhookTokensSection ──────────────────────────────────────────────────────

function WebhookTokensSection({ tenant }: { tenant: string }) {
  const { data: rawTokens, isLoading } = useWebhookTokens(tenant)
  const revokeToken = useRevokeToken(tenant)

  const [showGenerate, setShowGenerate] = useState(false)
  const [revealToken, setRevealToken] = useState<string | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<WebhookToken | null>(null)

  const tokens = Array.isArray(rawTokens) ? rawTokens : []

  function handleConfirmRevoke() {
    if (!revokeTarget) return
    revokeToken.mutate(revokeTarget.id, {
      onSuccess: () => {
        showToast('Токен отозван', 'success')
        setRevokeTarget(null)
      },
      onError: () => {
        showToast('Не удалось отозвать токен')
        setRevokeTarget(null)
      },
    })
  }

  return (
    <section className="rounded-lg border p-5">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <KeyRound size={16} className="text-muted-foreground" />
          <h2 className="text-sm font-semibold">Webhook-токены</h2>
        </div>
        <Button size="sm" variant="outline" onClick={() => setShowGenerate(true)}>
          <Plus size={13} className="mr-1" />
          Сгенерировать токен
        </Button>
      </div>

      {isLoading ? (
        <p className="py-6 text-center text-xs text-muted-foreground">Загрузка...</p>
      ) : tokens.length === 0 ? (
        <p className="py-6 text-center text-xs text-muted-foreground">Нет активных токенов.</p>
      ) : (
        <div className="flex flex-col divide-y">
          {tokens.map((tok) => (
            <div key={tok.id} className="flex items-center justify-between py-2.5">
              <div>
                <p className="font-mono text-xs font-medium">{tok.source}</p>
                <p className="text-xs text-muted-foreground">Создан {formatDate(tok.created_at)}</p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setRevokeTarget(tok)}
                className="text-destructive hover:text-destructive"
              >
                <Trash2 size={13} />
              </Button>
            </div>
          ))}
        </div>
      )}

      {showGenerate && (
        <GenerateTokenModal
          tenant={tenant}
          onClose={() => setShowGenerate(false)}
          onSuccess={(token) => {
            setShowGenerate(false)
            setRevealToken(token)
          }}
        />
      )}

      {revealToken && (
        <OneTimeTokenRevealModal
          token={revealToken}
          onClose={() => setRevealToken(null)}
        />
      )}

      {revokeTarget && (
        <RevokeConfirmDialog
          token={revokeTarget}
          onCancel={() => setRevokeTarget(null)}
          onConfirm={handleConfirmRevoke}
          isPending={revokeToken.isPending}
        />
      )}
    </section>
  )
}

// ── NotificationConfigSection ─────────────────────────────────────────────────

function NotificationConfigSection({ tenant }: { tenant: string }) {
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

// ── MembersSection ────────────────────────────────────────────────────────────

function roleLabel(role: string) {
  return role === 'admin' ? 'Администратор' : 'Участник'
}

function MembersSection({ tenant }: { tenant: string }) {
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

// ── TenantSettingsPage ────────────────────────────────────────────────────────

export function TenantSettingsPage() {
  const { tenant } = useParams<{ tenant: string }>()
  const t = tenant!

  return (
    <div className="flex h-full flex-col gap-5 overflow-auto p-4">
      <h1 className="text-base font-semibold">Настройки тенанта</h1>
      <WebhookTokensSection tenant={t} />
      <NotificationConfigSection tenant={t} />
      <MembersSection tenant={t} />
    </div>
  )
}

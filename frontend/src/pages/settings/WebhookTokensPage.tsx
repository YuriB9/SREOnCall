import { AlertTriangle, Check, ClipboardCopy, KeyRound, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useParams } from 'react-router-dom'

import { useCreateToken, useRevokeToken, useWebhookTokens } from '@/api/tenantSettings'
import type { WebhookToken } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

const VALID_SOURCES = ['alertmanager', 'grafana'] as const
type ValidSource = (typeof VALID_SOURCES)[number]

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  })
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

export function WebhookTokensSection({ tenant }: { tenant: string }) {
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

// ── WebhookTokensPage ─────────────────────────────────────────────────────────

export function WebhookTokensPage() {
  const { tenant } = useParams<{ tenant: string }>()

  return (
    <div className="flex h-full flex-col gap-5 overflow-auto p-4">
      <h1 className="text-base font-semibold">Webhook-токены</h1>
      <WebhookTokensSection tenant={tenant!} />
    </div>
  )
}

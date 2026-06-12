import { AlertCircle, ChevronLeft } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'

import { type ContactsInput, useSaveUserContacts, useUserContacts } from '@/api/profile'
import type { NotificationChannel } from '@/api/types'
import { useAuth } from '@/auth/useAuth'
import { usePermissions } from '@/auth/usePermissions'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

// ── Helpers ───────────────────────────────────────────────────────────────────

function isValidEmail(val: string) {
  if (!val) return true
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val)
}

const CHANNEL_LABELS: Record<NotificationChannel, string> = {
  email: 'Email',
  mattermost: 'Mattermost',
}

// ── ContactsForm ──────────────────────────────────────────────────────────────

interface ContactsFormProps {
  tenant: string
  userId: string
  defaultEmail?: string
}

function ContactsForm({ tenant, userId, defaultEmail }: ContactsFormProps) {
  const { data: serverContacts, isLoading } = useUserContacts(tenant, userId)
  const saveContacts = useSaveUserContacts(tenant, userId)

  const [email, setEmail] = useState('')
  const [mattermostUsername, setMattermostUsername] = useState('')
  const [enabledChannels, setEnabledChannels] = useState<NotificationChannel[]>([])
  const [emailWarning, setEmailWarning] = useState<string | null>(null)
  const [mattermostWarning, setMattermostWarning] = useState<string | null>(null)
  const initialized = useRef(false)

  useEffect(() => {
    if (isLoading) return
    if (initialized.current) return
    initialized.current = true
    // An existing contact has a non-empty id; the backend returns an empty
    // default (id === '') when nothing is configured yet.
    if (serverContacts?.id) {
      setEmail(serverContacts.email ?? '')
      setMattermostUsername(serverContacts.mattermost_username ?? '')
      setEnabledChannels((serverContacts.enabled_channels ?? []) as NotificationChannel[])
    } else if (defaultEmail) {
      // First visit: prefill the email from the Keycloak token and enable the
      // email channel so the user only needs to confirm with Save.
      setEmail(defaultEmail)
      setEnabledChannels(['email'])
    }
  }, [serverContacts, isLoading, defaultEmail])

  function buildPayload(): ContactsInput {
    return {
      email,
      mattermost_username: mattermostUsername,
      enabled_channels: enabledChannels,
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    saveContacts.mutate(buildPayload(), {
      onSuccess: () => showToast('Контакты сохранены', 'success'),
      onError: () => showToast('Не удалось сохранить контакты'),
    })
  }

  function handleToggleChannel(ch: NotificationChannel, enabled: boolean) {
    if (enabled && ch === 'email' && !email.trim()) {
      setEmailWarning('Укажите email перед включением этого канала')
      return
    }
    if (enabled && ch === 'email' && !isValidEmail(email)) {
      setEmailWarning('Укажите корректный email перед включением этого канала')
      return
    }
    setEmailWarning(null)
    if (enabled && ch === 'mattermost' && !mattermostUsername.trim()) {
      setMattermostWarning('Укажите имя пользователя в Mattermost перед включением этого канала')
      return
    }
    setMattermostWarning(null)
    const next: NotificationChannel[] = enabled
      ? [...enabledChannels, ch]
      : enabledChannels.filter((c) => c !== ch)

    setEnabledChannels(next)
    saveContacts.mutate(
      { email, mattermost_username: mattermostUsername, enabled_channels: next },
      {
        onSuccess: () => showToast(`Канал ${CHANNEL_LABELS[ch]} ${enabled ? 'включён' : 'отключён'}`, 'success'),
        onError: () => {
          setEnabledChannels(enabledChannels)
          showToast('Не удалось обновить настройки канала')
        },
      },
    )
  }

  if (isLoading) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Загрузка...</p>
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Contact info form */}
      <form onSubmit={handleSubmit} className="flex max-w-sm flex-col gap-4">
        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Email</label>
          <input
            type="text"
            value={email}
            readOnly
            disabled
            placeholder="you@example.com"
            className="w-full cursor-not-allowed rounded-md border bg-muted px-3 py-1.5 text-sm text-muted-foreground outline-none"
          />
          <p className="text-xs text-muted-foreground">Email берётся из учётной записи Keycloak и не редактируется здесь.</p>
        </div>

        <div className="space-y-1">
          <label className="text-xs font-medium text-muted-foreground">Имя пользователя в Mattermost</label>
          <input
            type="text"
            value={mattermostUsername}
            onChange={(e) => {
              setMattermostUsername(e.target.value)
              if (mattermostWarning) setMattermostWarning(null)
            }}
            placeholder="@username"
            className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <Button type="submit" size="sm" disabled={saveContacts.isPending} className="self-start">
          {saveContacts.isPending ? 'Сохранение...' : 'Сохранить контакты'}
        </Button>
      </form>

      {/* Notification channels */}
      <div className="flex flex-col gap-3">
        <p className="text-xs font-medium text-muted-foreground">Каналы уведомлений</p>

        {(['email', 'mattermost'] as NotificationChannel[]).map((ch) => (
          <div key={ch} className="flex flex-col gap-1">
            <label className="flex cursor-pointer items-center justify-between gap-3 rounded-md border p-3">
              <span className="text-sm font-medium">{CHANNEL_LABELS[ch]}</span>
              <button
                type="button"
                role="switch"
                aria-checked={enabledChannels.includes(ch)}
                onClick={() => handleToggleChannel(ch, !enabledChannels.includes(ch))}
                disabled={saveContacts.isPending}
                className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 ${
                  enabledChannels.includes(ch) ? 'bg-primary' : 'bg-input'
                }`}
              >
                <span
                  className={`pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg transition-transform ${
                    enabledChannels.includes(ch) ? 'translate-x-4' : 'translate-x-0'
                  }`}
                />
              </button>
            </label>
            {ch === 'email' && emailWarning && (
              <div className="flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300">
                <AlertCircle size={13} className="mt-0.5 shrink-0" />
                <span>{emailWarning}</span>
              </div>
            )}
            {ch === 'mattermost' && mattermostWarning && (
              <div className="flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300">
                <AlertCircle size={13} className="mt-0.5 shrink-0" />
                <span>{mattermostWarning}</span>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// ── ProfilePage ───────────────────────────────────────────────────────────────

export function ProfilePage() {
  const { user } = useAuth()
  const permissions = usePermissions()
  const navigate = useNavigate()

  const userId = user?.profile.sub ?? ''
  const tenants = Object.keys(permissions)

  const [selectedTenant, setSelectedTenant] = useState<string>('')

  useEffect(() => {
    if (!selectedTenant && tenants.length > 0) {
      setSelectedTenant(tenants[0])
    }
  }, [tenants, selectedTenant])

  return (
    <div className="flex h-full flex-col overflow-auto p-4">
      <div className="mb-4 flex items-center gap-2">
        <button
          type="button"
          onClick={() => navigate(-1)}
          className="rounded p-1 text-muted-foreground hover:bg-muted"
        >
          <ChevronLeft size={18} />
        </button>
        <h1 className="text-base font-semibold">Мой профиль</h1>
      </div>

      <div className="flex max-w-sm flex-col gap-5">
        <div className="rounded-lg border p-2 px-3">
          <p className="text-xs text-muted-foreground">Пользователь</p>
          <p className="text-sm font-medium">{user?.profile.preferred_username ?? userId}</p>
        </div>

        {tenants.length > 1 && (
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Команда</label>
            <select
              value={selectedTenant}
              onChange={(e) => setSelectedTenant(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              {tenants.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
        )}

        {selectedTenant && userId ? (
          <ContactsForm
            key={selectedTenant}
            tenant={selectedTenant}
            userId={userId}
            defaultEmail={user?.profile.email}
          />
        ) : tenants.length === 0 ? (
          <p className="text-sm text-muted-foreground">Вы не состоите ни в одной команде.</p>
        ) : null}
      </div>
    </div>
  )
}

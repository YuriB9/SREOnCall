import { AlertTriangle, CheckCircle2, Plus, Star, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import {
  useDefaultPolicy,
  useDeletePolicy,
  useEscalationPolicies,
  useSetDefaultPolicy,
} from '@/api/escalations'
import type { EscalationPolicy } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

// ── DeleteConfirmDialog ───────────────────────────────────────────────────────

interface DeleteDialogProps {
  policy: EscalationPolicy
  onCancel: () => void
  onConfirm: () => void
  isPending: boolean
}

function DeleteConfirmDialog({ policy, onCancel, onConfirm, isPending }: DeleteDialogProps) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="w-full max-w-sm rounded-xl border bg-background p-6 shadow-xl">
        <div className="mb-4 flex items-start gap-3">
          <AlertTriangle size={20} className="mt-0.5 shrink-0 text-destructive" />
          <div>
            <h2 className="text-sm font-semibold">Удалить политику?</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Политика <span className="font-medium">{policy.name}</span> будет удалена. Это действие необратимо.
            </p>
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={isPending}>
            Отмена
          </Button>
          <Button variant="destructive" size="sm" onClick={onConfirm} disabled={isPending}>
            {isPending ? 'Удаление...' : 'Удалить'}
          </Button>
        </div>
      </div>
    </div>
  )
}

// ── PolicyCard ────────────────────────────────────────────────────────────────

interface PolicyCardProps {
  policy: EscalationPolicy
  isDefault: boolean
  onEdit: () => void
  onDelete: () => void
  onSetDefault: () => void
  isSettingDefault: boolean
}

function PolicyCard({ policy, isDefault, onEdit, onDelete, onSetDefault, isSettingDefault }: PolicyCardProps) {
  const tierCount = Array.isArray(policy.tiers) ? policy.tiers.length : 0
  return (
    <div className="flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-medium">{policy.name}</span>
          {isDefault && (
            <span className="flex shrink-0 items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
              <CheckCircle2 size={11} />
              По умолчанию
            </span>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          {tierCount === 0 ? 'Нет уровней' : `${tierCount} ${tierLabel(tierCount)}`}
        </p>
      </div>

      <div className="flex shrink-0 flex-wrap items-center gap-2">
        {!isDefault && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onSetDefault}
            disabled={isSettingDefault}
            title="Сделать по умолчанию"
            className="gap-1 text-xs"
          >
            <Star size={13} />
            По умолчанию
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={onEdit}>
          Редактировать
        </Button>
        {!isDefault && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onDelete}
            className="text-destructive hover:text-destructive"
          >
            <Trash2 size={14} />
          </Button>
        )}
      </div>
    </div>
  )
}

function tierLabel(n: number): string {
  if (n % 10 === 1 && n % 100 !== 11) return 'уровень'
  if (n % 10 >= 2 && n % 10 <= 4 && !(n % 100 >= 12 && n % 100 <= 14)) return 'уровня'
  return 'уровней'
}

// ── EscalationPoliciesPage ────────────────────────────────────────────────────

export function EscalationPoliciesPage() {
  const { tenant } = useParams<{ tenant: string }>()
  const t = tenant!
  const navigate = useNavigate()

  const [deleteTarget, setDeleteTarget] = useState<EscalationPolicy | null>(null)

  const { data: rawPolicies, isLoading } = useEscalationPolicies(t)
  const { data: defaultConfig } = useDefaultPolicy(t)

  const policies = Array.isArray(rawPolicies) ? rawPolicies : []
  const defaultPolicyId = defaultConfig?.default_policy_id ?? null

  const deletePolicy = useDeletePolicy(t)
  const setDefaultPolicy = useSetDefaultPolicy(t)

  function handleConfirmDelete() {
    if (!deleteTarget) return
    deletePolicy.mutate(deleteTarget.id, {
      onSuccess: () => {
        showToast('Политика удалена', 'success')
        setDeleteTarget(null)
      },
      onError: () => {
        showToast('Не удалось удалить политику')
        setDeleteTarget(null)
      },
    })
  }

  function handleSetDefault(policy: EscalationPolicy) {
    setDefaultPolicy.mutate(policy.id, {
      onSuccess: () => showToast('Политика по умолчанию обновлена', 'success'),
      onError: () => showToast('Не удалось обновить политику по умолчанию'),
    })
  }

  return (
    <div className="flex h-full flex-col gap-4 overflow-auto p-4">
      <div className="flex items-center justify-between">
        <h1 className="text-base font-semibold">Политики эскалации</h1>
        <Button size="sm" onClick={() => navigate(`/${t}/escalations/new`)}>
          <Plus size={14} className="mr-1" />
          Новая политика
        </Button>
      </div>

      {isLoading ? (
        <p className="py-12 text-center text-sm text-muted-foreground">Загрузка...</p>
      ) : policies.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">
          Политик эскалации нет. Создайте первую.
        </p>
      ) : (
        <div className="flex flex-col gap-3">
          {policies.map((policy) => (
            <PolicyCard
              key={policy.id}
              policy={policy}
              isDefault={policy.id === defaultPolicyId}
              onEdit={() => navigate(`/${t}/escalations/${policy.id}/edit`)}
              onDelete={() => setDeleteTarget(policy)}
              onSetDefault={() => handleSetDefault(policy)}
              isSettingDefault={setDefaultPolicy.isPending}
            />
          ))}
        </div>
      )}

      {deleteTarget && (
        <DeleteConfirmDialog
          policy={deleteTarget}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={handleConfirmDelete}
          isPending={deletePolicy.isPending}
        />
      )}
    </div>
  )
}

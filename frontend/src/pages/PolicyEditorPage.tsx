import { ArrowDown, ArrowUp, ChevronLeft, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'

import {
  useCreatePolicy,
  useDefaultPolicy,
  useEscalationPolicies,
  useReplacePolicy,
} from '@/api/escalations'
import { useSchedules } from '@/api/schedules'
import type { PolicyTier } from '@/api/types'
import { Button } from '@/components/ui/button'
import { showToast } from '@/lib/toast'

// ── Local draft step ──────────────────────────────────────────────────────────

interface DraftStep {
  notify_schedule_id: string
  timeout_seconds: number
  error?: string
}

function emptyStep(): DraftStep {
  return { notify_schedule_id: '', timeout_seconds: 900 }
}

function stepsFromTiers(tiers: PolicyTier[]): DraftStep[] {
  return tiers
    .slice()
    .sort((a, b) => a.tier_number - b.tier_number)
    .map((t) => ({ notify_schedule_id: t.notify_schedule_id, timeout_seconds: t.timeout_seconds }))
}

// ── StepCard ──────────────────────────────────────────────────────────────────

interface StepCardProps {
  index: number
  total: number
  step: DraftStep
  scheduleOptions: { id: string; name: string }[]
  onChange: (updated: DraftStep) => void
  onMoveUp: () => void
  onMoveDown: () => void
  onRemove: () => void
}

function StepCard({
  index,
  total,
  step,
  scheduleOptions,
  onChange,
  onMoveUp,
  onMoveDown,
  onRemove,
}: StepCardProps) {
  const minutes = Math.round(step.timeout_seconds / 60)

  // The tier may reference a schedule that has since been deleted. Surface it as
  // an explicit option so the field doesn't silently look empty and so the user
  // can tell a dangling reference apart from "not chosen yet".
  const isDangling =
    Boolean(step.notify_schedule_id) &&
    !scheduleOptions.some((s) => s.id === step.notify_schedule_id)

  return (
    <div className="relative flex gap-3">
      <div className="flex flex-col items-center">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-xs font-semibold">
          {index + 1}
        </div>
        {index < total - 1 && <div className="mt-1 w-px flex-1 bg-border" />}
      </div>

      <div className="mb-4 flex-1 rounded-lg border p-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start">
          {/* Schedule dropdown */}
          <div className="flex-1 space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Расписание</label>
            <select
              value={step.notify_schedule_id}
              onChange={(e) =>
                onChange({ ...step, notify_schedule_id: e.target.value, error: undefined })
              }
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">Выберите расписание</option>
              {scheduleOptions.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
              {isDangling && (
                <option value={step.notify_schedule_id}>Расписание удалено — выберите заново</option>
              )}
            </select>
            {isDangling && !step.error && (
              <p className="text-xs text-destructive">
                Расписание было удалено. Выберите другое.
              </p>
            )}
            {step.error && <p className="text-xs text-destructive">{step.error}</p>}
          </div>

          {/* Timeout in minutes */}
          <div className="w-full space-y-1 sm:w-36">
            <label className="text-xs font-medium text-muted-foreground">Таймаут (мин)</label>
            <input
              type="number"
              min={1}
              value={minutes}
              onChange={(e) =>
                onChange({
                  ...step,
                  timeout_seconds: Math.max(1, parseInt(e.target.value) || 1) * 60,
                })
              }
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          {/* Reorder / remove */}
          <div className="flex items-start gap-1 pt-5">
            <button
              type="button"
              onClick={onMoveUp}
              disabled={index === 0}
              title="Переместить вверх"
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
            >
              <ArrowUp size={14} />
            </button>
            <button
              type="button"
              onClick={onMoveDown}
              disabled={index === total - 1}
              title="Переместить вниз"
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
            >
              <ArrowDown size={14} />
            </button>
            <button
              type="button"
              onClick={onRemove}
              disabled={total <= 1}
              title="Удалить шаг"
              className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-destructive disabled:opacity-30"
            >
              <Trash2 size={14} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── PolicyEditorPage ──────────────────────────────────────────────────────────

export function PolicyEditorPage() {
  const { tenant, policyId } = useParams<{ tenant: string; policyId?: string }>()
  const t = tenant!
  const isEdit = Boolean(policyId)
  const navigate = useNavigate()

  const [name, setName] = useState('')
  const [steps, setSteps] = useState<DraftStep[]>([emptyStep()])
  const [nameError, setNameError] = useState<string | null>(null)
  const [initialized, setInitialized] = useState(!isEdit)

  const { data: rawSchedules } = useSchedules(t)
  const scheduleOptions = Array.isArray(rawSchedules)
    ? rawSchedules.map((s) => ({ id: s.id, name: s.name }))
    : []

  const { data: rawPolicies } = useEscalationPolicies(t)
  const { data: defaultConfig } = useDefaultPolicy(t)
  const defaultPolicyId = defaultConfig?.default_policy_id ?? null

  useEffect(() => {
    if (!isEdit || initialized) return
    const policies = Array.isArray(rawPolicies) ? rawPolicies : []
    const policy = policies.find((p) => p.id === policyId)
    if (!policy) return
    setName(policy.name)
    const tiers = Array.isArray(policy.tiers) && policy.tiers.length > 0
      ? stepsFromTiers(policy.tiers)
      : [emptyStep()]
    setSteps(tiers)
    setInitialized(true)
  }, [rawPolicies, policyId, isEdit, initialized])

  const createPolicy = useCreatePolicy(t)
  const replacePolicy = useReplacePolicy(t, policyId ?? '', policyId === defaultPolicyId)

  function validate(): boolean {
    let valid = true
    if (!name.trim()) {
      setNameError('Введите название политики')
      valid = false
    } else {
      setNameError(null)
    }
    const updated = steps.map((s) => {
      if (!s.notify_schedule_id) {
        valid = false
        return { ...s, error: 'Выберите расписание' }
      }
      return { ...s, error: undefined }
    })
    setSteps(updated)
    return valid
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!validate()) return

    const payload = {
      name: name.trim(),
      tiers: steps.map((s, i) => ({
        tier_number: i + 1,
        timeout_seconds: s.timeout_seconds,
        notify_schedule_id: s.notify_schedule_id,
      })),
    }

    if (isEdit) {
      replacePolicy.mutate(payload, {
        onSuccess: () => {
          showToast('Политика обновлена', 'success')
          navigate(`/${t}/escalations`)
        },
        onError: () => showToast('Не удалось обновить политику'),
      })
    } else {
      createPolicy.mutate(payload, {
        onSuccess: () => {
          showToast('Политика создана', 'success')
          navigate(`/${t}/escalations`)
        },
        onError: () => showToast('Не удалось создать политику'),
      })
    }
  }

  function addStep() {
    setSteps((prev) => [...prev, emptyStep()])
  }

  function updateStep(i: number, updated: DraftStep) {
    setSteps((prev) => prev.map((s, idx) => (idx === i ? updated : s)))
  }

  function moveStep(from: number, to: number) {
    setSteps((prev) => {
      const next = [...prev]
      ;[next[from], next[to]] = [next[to], next[from]]
      return next
    })
  }

  function removeStep(i: number) {
    setSteps((prev) => prev.filter((_, idx) => idx !== i))
  }

  const isSaving = isEdit ? replacePolicy.isPending : createPolicy.isPending

  return (
    <div className="flex h-full flex-col overflow-auto p-4">
      <div className="mb-4 flex items-center gap-2">
        <button
          type="button"
          onClick={() => navigate(`/${t}/escalations`)}
          className="rounded p-1 text-muted-foreground hover:bg-muted"
        >
          <ChevronLeft size={18} />
        </button>
        <h1 className="text-base font-semibold">
          {isEdit ? 'Редактировать политику' : 'Новая политика'}
        </h1>
      </div>

      {isEdit && !initialized ? (
        <p className="py-12 text-center text-sm text-muted-foreground">Загрузка...</p>
      ) : (
        <form onSubmit={handleSubmit} className="flex max-w-xl flex-col gap-5">
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Название политики</label>
            <input
              type="text"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                if (nameError) setNameError(null)
              }}
              placeholder="Например: Основная эскалация"
              className="w-full rounded-md border bg-background px-3 py-1.5 text-sm outline-none focus:ring-1 focus:ring-ring"
            />
            {nameError && <p className="text-xs text-destructive">{nameError}</p>}
          </div>

          <div>
            <p className="mb-3 text-xs font-medium text-muted-foreground">Уровни эскалации</p>
            <div className="flex flex-col">
              {steps.map((step, i) => (
                <StepCard
                  key={i}
                  index={i}
                  total={steps.length}
                  step={step}
                  scheduleOptions={scheduleOptions}
                  onChange={(updated) => updateStep(i, updated)}
                  onMoveUp={() => moveStep(i, i - 1)}
                  onMoveDown={() => moveStep(i, i + 1)}
                  onRemove={() => removeStep(i)}
                />
              ))}
            </div>
            <button
              type="button"
              onClick={addStep}
              className="mt-1 flex items-center gap-1.5 rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground hover:border-primary hover:text-primary"
            >
              <Plus size={13} />
              Добавить уровень
            </button>
          </div>

          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => navigate(`/${t}/escalations`)}
              disabled={isSaving}
            >
              Отмена
            </Button>
            <Button type="submit" size="sm" disabled={isSaving}>
              {isSaving ? 'Сохранение...' : 'Сохранить'}
            </Button>
          </div>
        </form>
      )}
    </div>
  )
}

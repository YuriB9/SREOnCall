import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'

import { apiClient } from './client'
import type { Override, Schedule } from './types'

// ─── Query keys ───────────────────────────────────────────────────────────────

export function scheduleKeys(tenant: string) {
  return {
    all: [tenant, 'schedules'] as const,
    list: () => [tenant, 'schedules', 'list'] as const,
    oncall: (scheduleId: string) => [tenant, 'schedules', scheduleId, 'oncall'] as const,
    window: (scheduleId: string, from: string, to: string) =>
      [tenant, 'schedules', scheduleId, 'window', from, to] as const,
    overrides: (scheduleId: string) => [tenant, 'schedules', scheduleId, 'overrides'] as const,
  }
}

// ─── Queries ──────────────────────────────────────────────────────────────────

export function useSchedules(tenant: string) {
  return useQuery({
    queryKey: scheduleKeys(tenant).list(),
    queryFn: async () => {
      const { data } = await apiClient.get<Schedule[]>(`/schedules/v1/${tenant}/schedules`)
      return data
    },
  })
}

// ─── Conflict error ───────────────────────────────────────────────────────────

export interface ConflictDetail {
  existing_start: string
  existing_end: string
  existing_user: string
}

export class OverrideConflictError extends Error {
  detail: ConflictDetail
  constructor(detail: ConflictDetail) {
    super('Override conflicts with an existing override')
    this.detail = detail
    this.name = 'OverrideConflictError'
  }
}

function extractConflict(err: unknown): never {
  if (axios.isAxiosError(err) && err.response?.status === 409) {
    throw new OverrideConflictError(err.response.data as ConflictDetail)
  }
  throw err
}

// ─── Validation error (422) ─────────────────────────────────────────────────────

export class ScheduleValidationError extends Error {
  missingFields?: string[]
  constructor(message: string, missingFields?: string[]) {
    super(message)
    this.name = 'ScheduleValidationError'
    this.missingFields = missingFields
  }
}

function extractValidation(err: unknown): never {
  if (axios.isAxiosError(err) && err.response?.status === 422) {
    const body = err.response.data as { missing_fields?: string[]; error?: string }
    throw new ScheduleValidationError(body.error ?? 'Проверьте заполнение полей', body.missing_fields)
  }
  throw err
}

// ─── Mutations ────────────────────────────────────────────────────────────────

export interface ScheduleInput {
  name: string
  timezone: string
  rotation: string[]
  shift_duration: string
  start_date: string
}

export function useCreateSchedule(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: ScheduleInput) => {
      try {
        const { data } = await apiClient.post<Schedule>(
          `/schedules/v1/${tenant}/schedules`,
          body,
        )
        return data
      } catch (err) {
        extractValidation(err)
      }
    },
    onSuccess: () => {
      // Invalidate the whole schedules subtree (list + per-schedule shift
      // windows + oncall), so the Gantt reflects rotation/duration changes
      // immediately, not just the schedules list.
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).all })
    },
  })
}

export function useUpdateSchedule(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, ...body }: ScheduleInput & { id: string }) => {
      try {
        const { data } = await apiClient.patch<Schedule>(
          `/schedules/v1/${tenant}/schedules/${id}`,
          body,
        )
        return data
      } catch (err) {
        extractValidation(err)
      }
    },
    onSuccess: () => {
      // Invalidate the whole schedules subtree (list + per-schedule shift
      // windows + oncall), so the Gantt reflects rotation/duration changes
      // immediately, not just the schedules list.
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).all })
    },
  })
}

export function useDeleteSchedule(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiClient.delete(`/schedules/v1/${tenant}/schedules/${id}`),
    onSuccess: () => {
      // Invalidate the whole schedules subtree (list + per-schedule shift
      // windows + oncall), so the Gantt reflects rotation/duration changes
      // immediately, not just the schedules list.
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).all })
    },
  })
}

export interface CreateOverrideInput {
  scheduleId: string
  user_id: string
  start_at: string
  end_at: string
}

export function useCreateOverride(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ scheduleId, ...body }: CreateOverrideInput) => {
      try {
        const { data } = await apiClient.post<Override>(
          `/schedules/v1/${tenant}/schedules/${scheduleId}/overrides`,
          body,
        )
        return data
      } catch (err) {
        extractConflict(err)
      }
    },
    onSuccess: (_data, { scheduleId }) => {
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).overrides(scheduleId) })
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).all })
    },
  })
}

export function useDeleteOverride(tenant: string, scheduleId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (overrideId: string) =>
      apiClient.delete(`/schedules/v1/${tenant}/schedules/${scheduleId}/overrides/${overrideId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).overrides(scheduleId) })
      qc.invalidateQueries({ queryKey: scheduleKeys(tenant).all })
    },
  })
}

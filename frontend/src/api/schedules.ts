import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'

import { apiClient } from './client'
import type { OnCallNow, Override, Schedule, ShiftWindow } from './types'

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

export function useOnCallNow(tenant: string, scheduleId: string) {
  return useQuery({
    queryKey: scheduleKeys(tenant).oncall(scheduleId),
    queryFn: async () => {
      const { data } = await apiClient.get<OnCallNow | null>(
        `/schedules/v1/${tenant}/schedules/${scheduleId}/oncall`,
      )
      return data
    },
    refetchInterval: 60_000,
    enabled: Boolean(scheduleId),
  })
}

export function useScheduleWindow(
  tenant: string,
  scheduleId: string,
  from: string,
  to: string,
) {
  return useQuery({
    queryKey: scheduleKeys(tenant).window(scheduleId, from, to),
    queryFn: async () => {
      const { data } = await apiClient.get<ShiftWindow[]>(
        `/schedules/v1/${tenant}/schedules/${scheduleId}/oncall`,
        { params: { from, to } },
      )
      return data
    },
    enabled: Boolean(scheduleId && from && to),
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

// ─── Mutations ────────────────────────────────────────────────────────────────

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

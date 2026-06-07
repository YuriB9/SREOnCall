import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type { EscalationPolicy, EscalationStep } from './types'

// ─── Query keys ───────────────────────────────────────────────────────────────

export function escalationKeys(tenant: string) {
  return {
    all: [tenant, 'escalations'] as const,
    list: () => [tenant, 'escalations', 'list'] as const,
    detail: (id: string) => [tenant, 'escalations', id] as const,
  }
}

// ─── Query ────────────────────────────────────────────────────────────────────

export function useEscalationPolicies(tenant: string) {
  return useQuery({
    queryKey: escalationKeys(tenant).list(),
    queryFn: async () => {
      const { data } = await apiClient.get<EscalationPolicy[]>(
        `/escalations/v1/${tenant}/policies`,
      )
      return data
    },
  })
}

// ─── Mutations ────────────────────────────────────────────────────────────────

export interface PolicyInput {
  name: string
  steps: EscalationStep[]
}

export function useCreatePolicy(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: PolicyInput) =>
      apiClient.post<EscalationPolicy>(`/escalations/v1/${tenant}/policies`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).list() })
    },
  })
}

export function useUpdatePolicy(tenant: string, policyId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: PolicyInput) =>
      apiClient.put<EscalationPolicy>(`/escalations/v1/${tenant}/policies/${policyId}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).list() })
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).detail(policyId) })
    },
  })
}

export function useDeletePolicy(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (policyId: string) =>
      apiClient.delete(`/escalations/v1/${tenant}/policies/${policyId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).list() })
    },
  })
}

export function useSetDefaultPolicy(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (policyId: string) =>
      apiClient.patch(`/escalations/v1/${tenant}/policies/${policyId}`, { is_default: true }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).list() })
    },
  })
}

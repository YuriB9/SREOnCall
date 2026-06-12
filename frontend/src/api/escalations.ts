import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type {
  EscalationHistoryEntry,
  EscalationPolicy,
  EscalationState,
  PolicyTier,
  TenantEscalationConfig,
} from './types'

// ─── Query keys ───────────────────────────────────────────────────────────────

export function escalationKeys(tenant: string) {
  return {
    all: [tenant, 'escalations'] as const,
    list: () => [tenant, 'escalations', 'list'] as const,
    defaultPolicy: () => [tenant, 'escalations', 'default'] as const,
    states: (ids: string[]) => [tenant, 'escalations', 'states', ids] as const,
    history: (incidentId: string) =>
      [tenant, 'escalations', 'history', incidentId] as const,
  }
}

// Fetches escalation states for the given incidents in a single bulk call and
// returns them as a Map keyed by incident_id. Disabled when the id list is empty
// and degrades gracefully to an empty map if the escalation service errors, so
// the incident list keeps working when escalation is unavailable.
export function useEscalationStates(tenant: string, incidentIds: string[]) {
  const sortedIds = [...incidentIds].sort()
  return useQuery({
    queryKey: escalationKeys(tenant).states(sortedIds),
    enabled: sortedIds.length > 0,
    refetchInterval: 12_000,
    queryFn: async () => {
      try {
        const { data } = await apiClient.get<EscalationState[]>(
          `/escalations/v1/${tenant}/incidents/state`,
          { params: { incident_ids: sortedIds.join(',') } },
        )
        const map = new Map<string, EscalationState>()
        if (Array.isArray(data)) {
          for (const s of data) map.set(s.incident_id, s)
        }
        return map
      } catch {
        return new Map<string, EscalationState>()
      }
    },
  })
}

// Fetches the escalation history (level triggers, advances, stops) for a single
// incident. Disabled until an incidentId is provided and degrades gracefully to
// an empty array if the escalation service errors, so the history tab keeps
// rendering the incident journal when escalation is unavailable.
export function useEscalationHistory(tenant: string, incidentId: string) {
  return useQuery({
    queryKey: escalationKeys(tenant).history(incidentId),
    enabled: Boolean(incidentId),
    queryFn: async () => {
      try {
        const { data } = await apiClient.get<EscalationHistoryEntry[]>(
          `/escalations/v1/${tenant}/incidents/${incidentId}/history`,
        )
        return Array.isArray(data) ? data : []
      } catch {
        return [] as EscalationHistoryEntry[]
      }
    },
  })
}

// ─── Queries ──────────────────────────────────────────────────────────────────

export function useEscalationPolicies(tenant: string) {
  return useQuery({
    queryKey: escalationKeys(tenant).list(),
    queryFn: async () => {
      const { data } = await apiClient.get<EscalationPolicy[]>(
        `/escalations/v1/${tenant}/policies`,
      )
      return Array.isArray(data) ? data : []
    },
  })
}

export function useDefaultPolicy(tenant: string) {
  return useQuery({
    queryKey: escalationKeys(tenant).defaultPolicy(),
    queryFn: async () => {
      try {
        const { data } = await apiClient.get<TenantEscalationConfig>(
          `/escalations/v1/${tenant}/default-policy`,
        )
        return data
      } catch {
        return null
      }
    },
  })
}

// ─── Mutations ────────────────────────────────────────────────────────────────

export interface PolicyInput {
  name: string
  tiers: Omit<PolicyTier, 'id' | 'policy_id'>[]
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

// The backend has no PUT /policies/:id. Edit is implemented as: POST new → if was
// default set new as default → DELETE old. If POST fails, nothing is lost.
export function useReplacePolicy(tenant: string, oldPolicyId: string, wasDefault: boolean) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: PolicyInput) => {
      const { data: created } = await apiClient.post<EscalationPolicy>(
        `/escalations/v1/${tenant}/policies`,
        body,
      )
      if (wasDefault) {
        await apiClient.put(`/escalations/v1/${tenant}/default-policy`, {
          policy_id: created.id,
        })
      }
      await apiClient.delete(`/escalations/v1/${tenant}/policies/${oldPolicyId}`)
      return created
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).list() })
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).defaultPolicy() })
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
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).defaultPolicy() })
    },
  })
}

export function useSetDefaultPolicy(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (policyId: string) =>
      apiClient.put(`/escalations/v1/${tenant}/default-policy`, { policy_id: policyId }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: escalationKeys(tenant).defaultPolicy() })
    },
  })
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type { Alert, Comment, HistoryEntry, Incident, IncidentListResponse, IncidentStatus } from './types'

export interface IncidentFilters {
  status?: string
  severity?: string
  source?: string
  cursor?: string
  limit?: number
}

export function incidentKeys(tenant: string) {
  return {
    all: [tenant, 'incidents'] as const,
    list: (filters: IncidentFilters) => [tenant, 'incidents', 'list', filters] as const,
    detail: (id: string) => [tenant, 'incidents', id] as const,
    alerts: (id: string) => [tenant, 'incidents', id, 'alerts'] as const,
    history: (id: string) => [tenant, 'incidents', id, 'history'] as const,
    comments: (id: string) => [tenant, 'incidents', id, 'comments'] as const,
  }
}

export function useIncidents(tenant: string, filters: IncidentFilters = {}) {
  return useQuery({
    queryKey: incidentKeys(tenant).list(filters),
    queryFn: async () => {
      const { data } = await apiClient.get<IncidentListResponse>(
        `/incidents/v1/${tenant}/incidents`,
        { params: filters },
      )
      return data
    },
    refetchInterval: 12_000,
  })
}

export function useIncident(tenant: string, id: string) {
  return useQuery({
    queryKey: incidentKeys(tenant).detail(id),
    queryFn: async () => {
      const { data } = await apiClient.get<Incident>(`/incidents/v1/${tenant}/incidents/${id}`)
      return data
    },
    enabled: Boolean(id),
  })
}

export function useIncidentAlerts(tenant: string, id: string) {
  return useQuery({
    queryKey: incidentKeys(tenant).alerts(id),
    queryFn: async () => {
      const { data } = await apiClient.get<Alert[]>(
        `/incidents/v1/${tenant}/incidents/${id}/alerts`,
      )
      return data
    },
    enabled: Boolean(id),
  })
}

export function useIncidentHistory(tenant: string, id: string) {
  return useQuery({
    queryKey: incidentKeys(tenant).history(id),
    queryFn: async () => {
      const { data } = await apiClient.get<HistoryEntry[]>(
        `/incidents/v1/${tenant}/incidents/${id}/history`,
      )
      return data
    },
    enabled: Boolean(id),
  })
}

export function useIncidentComments(tenant: string, id: string) {
  return useQuery({
    queryKey: incidentKeys(tenant).comments(id),
    queryFn: async () => {
      const { data } = await apiClient.get<Comment[]>(
        `/incidents/v1/${tenant}/incidents/${id}/comments`,
      )
      return data
    },
    enabled: Boolean(id),
  })
}

// ─── Mutations ────────────────────────────────────────────────────────────────

function usePatchIncidentStatus(tenant: string, status: IncidentStatus) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      const { data } = await apiClient.patch<Incident>(
        `/incidents/v1/${tenant}/incidents/${id}`,
        { status },
      )
      return data
    },

    onMutate: async (id: string) => {
      await qc.cancelQueries({ queryKey: incidentKeys(tenant).all })
      const prev = qc.getQueryData<Incident>(incidentKeys(tenant).detail(id))

      qc.setQueryData<Incident>(incidentKeys(tenant).detail(id), (old) =>
        old ? { ...old, status } : old,
      )
      qc.setQueriesData<IncidentListResponse>(
        { queryKey: incidentKeys(tenant).all },
        (old) =>
          old && Array.isArray(old.incidents)
            ? {
                ...old,
                incidents: old.incidents.map((inc) =>
                  inc.id === id ? { ...inc, status } : inc,
                ),
              }
            : old,
      )
      return { prev, id }
    },

    onError: (_err, id, ctx) => {
      if (ctx?.prev) {
        qc.setQueryData(incidentKeys(tenant).detail(id), ctx.prev)
      }
      qc.invalidateQueries({ queryKey: incidentKeys(tenant).all })
    },

    onSuccess: (updated, id) => {
      // Write the server response (with acknowledged_by/resolved_at filled in)
      // into both caches so the UI reflects it immediately, without a refetch.
      qc.setQueryData<Incident>(incidentKeys(tenant).detail(id), updated)
      qc.setQueriesData<IncidentListResponse>(
        { queryKey: incidentKeys(tenant).all },
        (old) =>
          old && Array.isArray(old.incidents)
            ? {
                ...old,
                incidents: old.incidents.map((inc) =>
                  inc.id === id ? updated : inc,
                ),
              }
            : old,
      )
    },
  })
}

export function useAcknowledgeIncident(tenant: string) {
  return usePatchIncidentStatus(tenant, 'acknowledged')
}

export function useResolveIncident(tenant: string) {
  return usePatchIncidentStatus(tenant, 'resolved')
}

export function usePostComment(tenant: string, incidentId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (text: string) =>
      apiClient.post<Comment>(`/incidents/v1/${tenant}/incidents/${incidentId}/comments`, { body: text }),

    onSuccess: () => {
      qc.invalidateQueries({ queryKey: incidentKeys(tenant).comments(incidentId) })
      qc.invalidateQueries({ queryKey: incidentKeys(tenant).history(incidentId) })
    },
  })
}

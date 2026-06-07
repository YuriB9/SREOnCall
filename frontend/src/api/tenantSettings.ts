import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type { Member, NotificationConfig, WebhookToken, WebhookTokenCreated } from './types'

// ─── Query keys ───────────────────────────────────────────────────────────────

export function tenantSettingsKeys(tenant: string) {
  return {
    tokens: () => [tenant, 'settings', 'tokens'] as const,
    notificationConfig: () => [tenant, 'settings', 'notification-config'] as const,
    members: () => [tenant, 'settings', 'members'] as const,
  }
}

// ─── Webhook Tokens ───────────────────────────────────────────────────────────

export function useWebhookTokens(tenant: string) {
  return useQuery({
    queryKey: tenantSettingsKeys(tenant).tokens(),
    queryFn: async () => {
      const { data } = await apiClient.get<WebhookToken[]>(
        `/schedules/v1/tenants/${tenant}/webhook-tokens`,
      )
      return data
    },
  })
}

export function useCreateToken(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (source: string) =>
      apiClient
        .post<WebhookTokenCreated>(`/schedules/v1/tenants/${tenant}/webhook-tokens`, { source })
        .then((r) => r.data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: tenantSettingsKeys(tenant).tokens() })
    },
  })
}

export function useRevokeToken(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (tokenId: string) =>
      apiClient.delete(`/schedules/v1/tenants/${tenant}/webhook-tokens/${tokenId}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: tenantSettingsKeys(tenant).tokens() })
    },
  })
}

// ─── Notification Config ──────────────────────────────────────────────────────

export function useNotificationConfig(tenant: string) {
  return useQuery({
    queryKey: tenantSettingsKeys(tenant).notificationConfig(),
    queryFn: async () => {
      const { data } = await apiClient.get<NotificationConfig>(
        `/schedules/v1/tenants/${tenant}/notification-config`,
      )
      return data
    },
  })
}

export function useSaveNotificationConfig(tenant: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: Partial<NotificationConfig>) =>
      apiClient
        .put<NotificationConfig>(`/schedules/v1/tenants/${tenant}/notification-config`, body)
        .then((r) => r.data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: tenantSettingsKeys(tenant).notificationConfig() })
    },
  })
}

// ─── Members ──────────────────────────────────────────────────────────────────

export function useMembers(tenant: string) {
  return useQuery({
    queryKey: tenantSettingsKeys(tenant).members(),
    queryFn: async () => {
      const { data } = await apiClient.get<Member[]>(`/schedules/v1/tenants/${tenant}/members`)
      return data
    },
  })
}

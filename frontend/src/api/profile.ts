import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type { NotificationChannel, UserContacts } from './types'

// Backend: GET/PUT /api/notifications/v1/{tenant}/contacts/{userId}
// (notification service, not scheduling service)

function contactsKey(tenant: string, userId: string) {
  return ['user', 'contacts', tenant, userId] as const
}

export interface ContactsInput {
  email: string
  mattermost_username: string
  enabled_channels: NotificationChannel[]
}

export function useUserContacts(tenant: string, userId: string) {
  return useQuery({
    queryKey: contactsKey(tenant, userId),
    queryFn: async () => {
      try {
        const { data } = await apiClient.get<UserContacts>(
          `/notifications/v1/${tenant}/contacts/${userId}`,
        )
        return data
      } catch {
        // 404 = no contacts configured yet
        return null
      }
    },
    enabled: Boolean(tenant && userId),
  })
}

export function useSaveUserContacts(tenant: string, userId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ContactsInput) =>
      apiClient
        .put<UserContacts>(`/notifications/v1/${tenant}/contacts/${userId}`, body)
        .then((r) => r.data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: contactsKey(tenant, userId) })
    },
  })
}

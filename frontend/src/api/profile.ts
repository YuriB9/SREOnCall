import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiClient } from './client'
import type { UserContacts } from './types'

const CONTACTS_KEY = ['user', 'contacts'] as const

export function useUserContacts() {
  return useQuery({
    queryKey: CONTACTS_KEY,
    queryFn: async () => {
      const { data } = await apiClient.get<UserContacts>('/schedules/v1/users/me/contacts')
      return data
    },
  })
}

export function useSaveUserContacts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: Partial<UserContacts>) =>
      apiClient.put<UserContacts>('/schedules/v1/users/me/contacts', body).then((r) => r.data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: CONTACTS_KEY })
    },
  })
}

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'

import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getProxyPool, saveProxyPool } from '../api'
import type { ProxyPoolEntry } from '../types'

export const PROXY_POOL_QUERY_KEY = ['proxy-pool', 'entries']

export function useProxyPool() {
  return useQuery({
    queryKey: PROXY_POOL_QUERY_KEY,
    queryFn: async () => {
      const res = requireServerSuccess(await getProxyPool())
      return res.data ?? []
    },
  })
}

export function useSaveProxyPool() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (entries: ProxyPoolEntry[]) => saveProxyPool(entries),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: PROXY_POOL_QUERY_KEY })
        queryClient.invalidateQueries({ queryKey: ['request-policy'] })
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        toast.success(i18next.t('Saved successfully'))
      } else {
        handleServerError(res)
      }
    },
    onError: (error: Error) => {
      handleServerError(error, i18next.t('Failed to save proxy pool'))
    },
  })
}

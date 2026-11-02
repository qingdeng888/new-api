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
import { api } from '@/lib/api'

import type { ProxyPoolEntry } from './types'

interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

/** Reads the global proxy pool (channel-scoped read endpoint). */
export async function getProxyPool(): Promise<ApiResponse<ProxyPoolEntry[]>> {
  const res = await api.get('/api/channel/proxy_pool')
  return res.data
}

/** Persists the whole pool document through the request-policy option. */
export async function saveProxyPool(
  entries: ProxyPoolEntry[]
): Promise<ApiResponse<unknown>> {
  const res = await api.patch('/api/option/request_policy', {
    options: { 'proxy_pool_setting.proxies': JSON.stringify(entries) },
  })
  return res.data
}

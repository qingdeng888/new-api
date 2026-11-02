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

import type { ProxyPool } from './types'

interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export async function getProxyPools(): Promise<ApiResponse<ProxyPool[]>> {
  const res = await api.get('/api/proxy_pool/')
  return res.data
}

export async function createProxyPool(data: {
  name: string
  proxies: string
}): Promise<ApiResponse<ProxyPool>> {
  const res = await api.post('/api/proxy_pool/', data)
  return res.data
}

export async function updateProxyPool(
  id: number,
  data: { name: string; proxies: string }
): Promise<ApiResponse<ProxyPool>> {
  const res = await api.put(`/api/proxy_pool/${id}`, data)
  return res.data
}

export async function deleteProxyPool(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/proxy_pool/${id}`)
  return res.data
}

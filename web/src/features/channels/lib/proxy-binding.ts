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

import type { ProxyPoolEntry } from '@/features/system-settings/proxy-manage/types'

export const PINNED_POOL_PREFIX = 'pool:url:'

/**
 * Bindings address the single global proxy pool:
 * "" inherits, "direct" dials without a proxy, "pool" rotates randomly over
 * the enabled entries, "pool:url:<url>" pins one entry, anything else is a
 * proxy URL. Legacy "pool:<n>" pins are still parsed.
 */
export type ProxyBindingMode = 'inherit' | 'direct' | 'pool' | 'pinned' | 'url'

export interface ParsedProxyBinding {
  mode: ProxyBindingMode
  pinnedUrl?: string
}

export function parseProxyBinding(
  value: string | undefined
): ParsedProxyBinding {
  const trimmed = value?.trim() || ''
  if (!trimmed) return { mode: 'inherit' }
  if (trimmed === 'direct') return { mode: 'direct' }
  if (trimmed === 'pool') return { mode: 'pool' }
  if (trimmed.startsWith(PINNED_POOL_PREFIX)) {
    const url = trimmed.slice(PINNED_POOL_PREFIX.length)
    if (url) return { mode: 'pinned', pinnedUrl: url }
  }
  // Legacy numeric pins are still recognised so old bindings keep working.
  const legacy = /^pool:(\d+)$/.exec(trimmed)
  if (legacy) return { mode: 'pinned', pinnedUrl: undefined }
  return { mode: 'url' }
}

export function buildRandomPoolBinding(): string {
  return 'pool'
}

export function buildPinnedPoolBinding(proxyUrl: string): string {
  return `${PINNED_POOL_PREFIX}${proxyUrl}`
}

export function isPoolBinding(value: string | undefined): boolean {
  const mode = parseProxyBinding(value).mode
  return mode === 'pool' || mode === 'pinned'
}

/** Label for a pool entry in a picker: "note · masked-url" or the masked URL. */
export function proxyEntryLabel(entry: ProxyPoolEntry): string {
  const masked = maskProxyUrl(entry.url)
  return entry.note ? `${entry.note} · ${masked}` : masked
}

function maskProxyUrl(url: string): string {
  const at = url.lastIndexOf('@')
  if (at <= 0) return url
  const schemeEnd = url.indexOf('://')
  const prefix = schemeEnd >= 0 ? url.slice(0, schemeEnd + 3) : ''
  return `${prefix}***@${url.slice(at + 1)}`
}

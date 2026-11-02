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

/**
 * Bindings address the single global proxy pool:
 * "" inherits, "direct" dials without a proxy, "pool" rotates randomly over
 * the pool, "pool:<n>" pins the n-th entry, anything else is a proxy URL.
 */
export type ProxyBindingMode = 'inherit' | 'direct' | 'pool' | 'pinned' | 'url'

export interface ParsedProxyBinding {
  mode: ProxyBindingMode
  pinnedIndex?: number
}

export function parseProxyBinding(
  value: string | undefined
): ParsedProxyBinding {
  const trimmed = value?.trim() || ''
  if (!trimmed) return { mode: 'inherit' }
  if (trimmed === 'direct') return { mode: 'direct' }
  if (trimmed === 'pool') return { mode: 'pool' }
  const rest = trimmed.startsWith('pool:') ? trimmed.slice(5) : null
  if (rest !== null) {
    const index = Number.parseInt(rest, 10)
    if (Number.isInteger(index) && index >= 0 && String(index) === rest) {
      return { mode: 'pinned', pinnedIndex: index }
    }
  }
  return { mode: 'url' }
}

export function buildPoolBinding(pinnedIndex?: number): string {
  return pinnedIndex === undefined ? 'pool' : `pool:${pinnedIndex}`
}

/** True when the binding references the global pool in any form. */
export function isPoolBinding(value: string | undefined): boolean {
  const mode = parseProxyBinding(value).mode
  return mode === 'pool' || mode === 'pinned'
}

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

/** Modes a proxy pool binding can use. */
export type ProxyPoolBindingMode = 'random' | 'fixed'

export interface ParsedProxyPoolBinding {
  poolId: number
  mode: ProxyPoolBindingMode
  fixedIndex?: number
}

/**
 * Parses a `pool:<id>:(random|fixed:<n>)` binding; returns null for
 * anything else (plain URLs, direct, empty).
 */
export function parseProxyPoolBinding(
  value: string | undefined
): ParsedProxyPoolBinding | null {
  const trimmed = value?.trim() || ''
  const rest = trimmed.startsWith('pool:') ? trimmed.slice(5) : null
  if (rest === null) return null
  const parts = rest.split(':')
  if (parts.length < 2) return null
  const poolId = Number.parseInt(parts[0], 10)
  if (!Number.isInteger(poolId) || poolId <= 0) return null
  switch (parts[1]) {
    case 'random':
      if (parts.length !== 2) return null
      return { poolId, mode: 'random' }
    case 'fixed': {
      if (parts.length !== 3) return null
      const fixedIndex = Number.parseInt(parts[2], 10)
      if (!Number.isInteger(fixedIndex) || fixedIndex < 0) return null
      return { poolId, mode: 'fixed', fixedIndex }
    }
    default:
      return null
  }
}

export function buildProxyPoolBinding(
  poolId: number,
  mode: ProxyPoolBindingMode,
  fixedIndex?: number
): string {
  if (mode === 'fixed') {
    return `pool:${poolId}:fixed:${fixedIndex ?? 0}`
  }
  return `pool:${poolId}:${mode}`
}

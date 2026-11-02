import { createSectionRegistry } from '../utils/section-registry'
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
import { ProxyPoolSection } from './proxy-pool-section'

type ProxyPoolSettings = Record<string, never>

const PROXY_POOL_SECTIONS = [
  {
    id: 'pools',
    titleKey: 'Proxy pools',
    build: () => <ProxyPoolSection />,
  },
] as const

export type ProxyPoolSectionId = (typeof PROXY_POOL_SECTIONS)[number]['id']

const proxyPoolRegistry = createSectionRegistry<
  ProxyPoolSectionId,
  ProxyPoolSettings
>({
  sections: PROXY_POOL_SECTIONS,
  defaultSection: 'pools',
  basePath: '/system-settings/proxy-pool',
  urlStyle: 'path',
})

export const PROXY_POOL_SECTION_IDS = proxyPoolRegistry.sectionIds
export const PROXY_POOL_DEFAULT_SECTION = proxyPoolRegistry.defaultSection
export const getProxyPoolSectionNavItems = proxyPoolRegistry.getSectionNavItems
export const getProxyPoolSectionContent = proxyPoolRegistry.getSectionContent
export const getProxyPoolSectionMeta = proxyPoolRegistry.getSectionMeta

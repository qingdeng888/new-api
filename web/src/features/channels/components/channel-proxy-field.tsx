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
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useProxyPool } from '@/features/system-settings/proxy-manage/hooks/use-proxy-pool'

import {
  buildPinnedPoolBinding,
  buildRandomPoolBinding,
  parseProxyBinding,
  proxyEntryLabel,
} from '../lib'

type ChannelProxyFieldProps = {
  value: string | undefined
  onChange: (value: string) => void
} & Omit<React.ComponentProps<'div'>, 'onChange'>

type FieldMode = 'inherit' | 'direct' | 'pool' | 'pinned' | 'manual'

/**
 * Channel-level proxy binding: inherit, dial directly, rotate through the
 * global proxy pool, pin one pool entry, or a manual proxy address.
 */
export function ChannelProxyField({
  value,
  onChange,
  ...rest
}: ChannelProxyFieldProps) {
  const { t } = useTranslation()
  const parsed = parseProxyBinding(value)
  const { data: entries = [] } = useProxyPool()
  const mode: FieldMode =
    parsed.mode === 'url' ? 'manual' : (parsed.mode as FieldMode)
  const invalid = rest['aria-invalid']
  const pinnedUrl = parsed.pinnedUrl ?? entries[0]?.url ?? ''

  const handleModeChange = (next: string | null) => {
    if (next === null) return
    switch (next) {
      case 'inherit':
        onChange('')
        break
      case 'direct':
        onChange('direct')
        break
      case 'pool':
        onChange(buildRandomPoolBinding())
        break
      case 'pinned':
        if (entries[0]) onChange(buildPinnedPoolBinding(entries[0].url))
        break
      default:
        onChange('')
    }
  }

  let hint: string
  switch (mode) {
    case 'manual':
      hint = t(
        'Network proxy for this channel (supports HTTP, HTTPS, SOCKS5, and SOCKS5H)'
      )
      break
    case 'pinned':
      hint = t(
        'Pin one proxy from the pool. Traffic moves to another healthy entry while it is failing.'
      )
      break
    case 'pool':
      hint = t(
        'Requests rotate randomly across the enabled proxies in the pool. Failing entries cool down and traffic transfers automatically.'
      )
      break
    case 'direct':
      hint = t('This channel dials upstream without any proxy.')
      break
    default:
      hint = t('This channel uses no channel-level proxy.')
  }

  return (
    <div {...rest} className='space-y-2' role='group' aria-label={t('Proxy Address')}>
      <Select
        value={mode}
        onValueChange={handleModeChange}
        items={[
          { value: 'inherit', label: t('Inherit (no channel proxy)') },
          { value: 'direct', label: t('Direct connection (no proxy)') },
          { value: 'pool', label: t('Global proxy pool (random)') },
          { value: 'pinned', label: t('Specific pool proxy') },
          { value: 'manual', label: t('Manual proxy address') },
        ]}
      >
        <SelectTrigger className='w-full sm:w-64'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            <SelectItem value='inherit'>
              {t('Inherit (no channel proxy)')}
            </SelectItem>
            <SelectItem value='direct'>
              {t('Direct connection (no proxy)')}
            </SelectItem>
            <SelectItem value='pool'>
              {t('Global proxy pool (random)')}
            </SelectItem>
            <SelectItem value='pinned'>{t('Specific pool proxy')}</SelectItem>
            <SelectItem value='manual'>{t('Manual proxy address')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      {mode === 'pinned' && (
        <Select
          value={pinnedUrl}
          onValueChange={(next) => {
            if (next === null) return
            onChange(buildPinnedPoolBinding(next))
          }}
          items={entries.map((entry) => ({
            value: entry.url,
            label: proxyEntryLabel(entry),
          }))}
        >
          <SelectTrigger className='w-full' aria-label={t('Pool proxy')}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {entries.map((entry) => (
                <SelectItem key={entry.url} value={entry.url}>
                  {proxyEntryLabel(entry)}
                  {!entry.enabled && ` (${t('Disabled')})`}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      )}

      <Input
        aria-invalid={invalid}
        aria-label={t('Proxy Address')}
        placeholder={t('socks5://user:pass@host:port')}
        spellCheck={false}
        disabled={mode !== 'manual'}
        value={mode === 'manual' ? (value ?? '') : ''}
        onChange={(event) => onChange(event.target.value)}
      />
      <p className='text-muted-foreground text-xs'>{hint}</p>
    </div>
  )
}

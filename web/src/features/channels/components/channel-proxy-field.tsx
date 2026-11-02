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

import { parseProxyBinding } from '../lib'

type ChannelProxyFieldProps = {
  value: string | undefined
  onChange: (value: string) => void
} & Omit<React.ComponentProps<'div'>, 'onChange'>

type FieldMode = 'inherit' | 'direct' | 'pool' | 'manual'

/**
 * Channel-level proxy binding: inherit, dial directly, rotate through the
 * global proxy pool, or a manual proxy address.
 */
export function ChannelProxyField({
  value,
  onChange,
  ...rest
}: ChannelProxyFieldProps) {
  const { t } = useTranslation()
  const parsed = parseProxyBinding(value)
  const mode: FieldMode =
    parsed.mode === 'pinned'
      ? 'pool'
      : parsed.mode === 'url'
        ? 'manual'
        : parsed.mode
  const invalid = rest['aria-invalid']

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
        onChange('pool')
        break
      default:
        onChange('')
    }
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
            <SelectItem value='manual'>{t('Manual proxy address')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      <Input
        aria-invalid={invalid}
        aria-label={t('Proxy Address')}
        placeholder={t('socks5://user:pass@host:port')}
        spellCheck={false}
        disabled={mode !== 'manual'}
        value={mode === 'manual' ? (value ?? '') : ''}
        onChange={(event) => onChange(event.target.value)}
      />
      <p className='text-muted-foreground text-xs'>
        {mode === 'manual'
          ? t(
              'Network proxy for this channel (supports HTTP, HTTPS, SOCKS5, and SOCKS5H)'
            )
          : mode === 'pool'
            ? t(
                'Requests rotate randomly across the proxies configured in System Settings → Request policies → Channel health. Failing entries cool down and traffic transfers automatically.'
              )
            : mode === 'direct'
              ? t('This channel dials upstream without any proxy.')
              : t('This channel uses no channel-level proxy.')}
      </p>
    </div>
  )
}

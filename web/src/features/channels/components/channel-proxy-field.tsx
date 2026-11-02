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
import { useProxyPools } from '@/features/system-settings/proxy-pool/hooks/use-proxy-pools'

import { buildProxyPoolBinding, parseProxyPoolBinding } from '../lib'

type ChannelProxyFieldProps = {
  value: string | undefined
  onChange: (value: string) => void
} & Omit<React.ComponentProps<'div'>, 'onChange'>

type ProxyFieldMode = 'manual' | 'pool'

/**
 * The channel-level proxy binding: a manual proxy address (including
 * "direct") or a proxy pool reference with a rotation mode.
 */
export function ChannelProxyField({
  value,
  onChange,
  ...rest
}: ChannelProxyFieldProps) {
  const { t } = useTranslation()
  const { data: pools = [] } = useProxyPools()
  const invalid = rest['aria-invalid']
  const parsed = parseProxyPoolBinding(value)
  const mode: ProxyFieldMode = parsed ? 'pool' : 'manual'
  const selectedPoolId = parsed?.poolId ?? pools[0]?.id ?? 0
  const selectedMode = parsed?.mode === 'ordered' ? 'ordered' : 'random'

  const handleModeChange = (next: string | null) => {
    if (next === null) return
    if (next === 'manual') {
      onChange('')
      return
    }
    if (pools.length === 0) {
      onChange('')
      return
    }
    onChange(buildProxyPoolBinding(selectedPoolId || pools[0].id, 'random'))
  }

  return (
    <div {...rest} className='space-y-2'>
      <Select
        value={mode}
        onValueChange={handleModeChange}
        items={[
          { value: 'manual', label: t('Manual proxy address') },
          { value: 'pool', label: t('Proxy pool') },
        ]}
      >
        <SelectTrigger className='w-full sm:w-56'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            <SelectItem value='manual'>{t('Manual proxy address')}</SelectItem>
            <SelectItem value='pool'>{t('Proxy pool')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      {mode === 'manual' ? (
        <>
          <Input
            aria-invalid={invalid}
            aria-label={t('Proxy Address')}
            placeholder={t('socks5://user:pass@host:port')}
            spellCheck={false}
            value={value ?? ''}
            onChange={(event) => onChange(event.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'Network proxy for this channel (supports HTTP, HTTPS, SOCKS5, and SOCKS5H). Enter direct to dial without a proxy.'
            )}
          </p>
        </>
      ) : pools.length === 0 ? (
        <p className='text-muted-foreground text-xs'>
          {t(
            'No proxy pools configured yet. Create one in System Settings → Proxy pools first.'
          )}
        </p>
      ) : (
        <div className='flex flex-wrap gap-2'>
          <Select
            value={String(selectedPoolId)}
            onValueChange={(next) => {
              if (next === null) return
              onChange(
                buildProxyPoolBinding(Number.parseInt(next, 10), selectedMode)
              )
            }}
            items={pools.map((pool) => ({
              value: String(pool.id),
              label: pool.name,
            }))}
          >
            <SelectTrigger className='w-full sm:w-56'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {pools.map((pool) => (
                  <SelectItem key={pool.id} value={String(pool.id)}>
                    {pool.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select
            value={selectedMode}
            onValueChange={(next) => {
              if (next === null) return
              onChange(
                buildProxyPoolBinding(
                  selectedPoolId,
                  next === 'ordered' ? 'ordered' : 'random'
                )
              )
            }}
            items={[
              { value: 'random', label: t('Random rotation') },
              { value: 'ordered', label: t('Ordered rotation') },
            ]}
          >
            <SelectTrigger className='w-full sm:w-40'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='random'>{t('Random rotation')}</SelectItem>
                <SelectItem value='ordered'>{t('Ordered rotation')}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
}

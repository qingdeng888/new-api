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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isOptionalProxyURL } from '@/features/channels/lib'

import { parseProxyPoolEntries } from '../types'
import type { ProxyPool } from '../types'

type PoolFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  pool: ProxyPool | null
  saving: boolean
  onSubmit: (values: { name: string; proxies: string }) => void
}

export function PoolFormDialog(props: PoolFormDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [proxies, setProxies] = useState('')

  useEffect(() => {
    if (props.open) {
      setName(props.pool?.name ?? '')
      setProxies(props.pool?.proxies ?? '')
    }
  }, [props.open, props.pool])

  const invalidLines = useMemo(() => {
    const invalid: number[] = []
    proxies.split('\n').forEach((line, index) => {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#')) return
      if (!isOptionalProxyURL(trimmed)) invalid.push(index + 1)
    })
    return invalid
  }, [proxies])

  const entryCount = parseProxyPoolEntries({
    id: 0,
    name,
    proxies,
    created_time: 0,
  }).length
  const nameInvalid = name.trim().length === 0 || name.trim().length > 64
  const canSubmit = !nameInvalid && invalidLines.length === 0 && entryCount > 0

  const handleSubmit = () => {
    if (!canSubmit || props.saving) return
    props.onSubmit({ name: name.trim(), proxies })
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.pool ? t('Edit proxy pool') : t('Create proxy pool')}
      description={t(
        'One proxy per line. Blank lines and # comments are ignored. HTTP, HTTPS, SOCKS5 and SOCKS5H are supported, with optional user:password credentials.'
      )}
      contentClassName='sm:max-w-xl'
      bodyClassName='space-y-3'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.saving}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || props.saving}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-3'>
        <div className='space-y-1.5'>
          <Label htmlFor='proxy-pool-name'>{t('Pool name')}</Label>
          <Input
            id='proxy-pool-name'
            value={name}
            maxLength={64}
            onChange={(event) => setName(event.target.value)}
            placeholder={t('Pool name')}
          />
        </div>
        <div className='space-y-1.5'>
          <div className='flex items-center justify-between'>
            <Label htmlFor='proxy-pool-proxies'>{t('Proxies')}</Label>
            <span className='text-muted-foreground text-xs'>
              {t('{{count}} proxies', { count: entryCount })}
            </span>
          </div>
          <textarea
            id='proxy-pool-proxies'
            value={proxies}
            onChange={(event) => setProxies(event.target.value)}
            rows={8}
            spellCheck={false}
            className='border-input bg-background placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 flex min-h-40 w-full rounded-md border px-3 py-2 font-mono text-xs shadow-xs outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50'
            placeholder={
              'http://user:pass@ip:port\nsocks5://ip:port\n# comment'
            }
          />
          {invalidLines.length > 0 && (
            <p className='text-destructive text-xs'>
              {t('Invalid proxy on line {{lines}}', {
                lines: invalidLines.join(', '),
              })}
            </p>
          )}
        </div>
      </div>
    </Dialog>
  )
}

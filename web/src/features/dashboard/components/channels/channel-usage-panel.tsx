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
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { ChevronRight } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'
import { getRollingDateRange } from '@/lib/time'
import { cn } from '@/lib/utils'

import { getChannelUsage } from '../../api'
import { TIME_RANGE_PRESETS } from '../../constants'
import type { ChannelUsageKey } from '../../types'
import { PanelWrapper } from '../ui/panel-wrapper'

const DEFAULT_RANGE_DAYS = 7

type UsageRow = {
  request_count: number
  success_count: number
  failure_count: number
  prompt_tokens: number
  completion_tokens: number
  cache_read_tokens: number
}

// UsageCells renders the metric columns shared by channel, key and model rows.
function UsageCells(props: { totals: UsageRow; locale: Intl.LocalesArgument }) {
  const cells: Array<[string, number]> = [
    ['requests', props.totals.request_count],
    ['success', props.totals.success_count],
    ['failed', props.totals.failure_count],
    ['prompt-tokens', props.totals.prompt_tokens],
    ['completion-tokens', props.totals.completion_tokens],
    ['cache-read-tokens', props.totals.cache_read_tokens],
  ]
  return (
    <>
      {cells.map(([column, value]) => (
        <td
          key={column}
          className='text-muted-foreground px-3 py-1.5 text-right tabular-nums'
        >
          {formatNumber(value, props.locale)}
        </td>
      ))}
    </>
  )
}

// channelUsageKeyLabel names a key row: its position and masked value for a
// current key, and a placeholder for a removed key or an attempt without one.
function channelUsageKeyLabel(key: ChannelUsageKey, t: TFunction): string {
  if (key.removed) {
    return t('Deleted key')
  }
  if (key.key_index < 0) {
    return t('Unassigned')
  }
  return `${t('Key')} #${key.key_index + 1} ${key.key_label}`
}

/**
 * ChannelUsagePanel is the channel statistics tab: every channel with the usage
 * of its keys, and the models each key served. A channel row expands to its
 * keys, which keeps the tab readable for installations with many keys.
 */
export function ChannelUsagePanel() {
  const { t, i18n } = useTranslation()
  const [rangeDays, setRangeDays] = useState(DEFAULT_RANGE_DAYS)
  const [expandedChannels, setExpandedChannels] = useState<Set<number>>(
    () => new Set()
  )
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['dashboard', 'channel-usage', rangeDays],
    queryFn: async () => {
      const { start, end } = getRollingDateRange(rangeDays)
      return requireServerSuccess(
        await getChannelUsage({
          start_timestamp: Math.floor(start.getTime() / 1000),
          end_timestamp: Math.floor(end.getTime() / 1000),
        })
      )
    },
    staleTime: 60_000,
  })

  const channels = data?.data?.channels ?? []

  const toggleChannel = useCallback((channelId: number) => {
    setExpandedChannels((previous) => {
      const next = new Set(previous)
      if (next.has(channelId)) {
        next.delete(channelId)
      } else {
        next.add(channelId)
      }
      return next
    })
  }, [])

  const headerCellClassName =
    'text-muted-foreground px-3 py-1.5 text-right font-medium'

  return (
    <div className='space-y-3'>
      <Tabs
        value={String(rangeDays)}
        onValueChange={(value) => setRangeDays(Number(value))}
        className='shrink-0'
      >
        <TabsList>
          {TIME_RANGE_PRESETS.map((preset) => (
            <TabsTrigger
              key={preset.days}
              value={String(preset.days)}
              className='px-2.5 text-xs'
            >
              {t(preset.label)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <PanelWrapper
        title={t('Channel Statistics')}
        description={t('Channel usage is aggregated from the last 30 days')}
        loading={isLoading}
        empty={!isLoading && !isError && channels.length === 0}
        emptyMessage={t('No data')}
      >
        {isError ? (
          <ErrorState
            description={getServerErrorMessage(error)}
            onRetry={() => void refetch()}
          />
        ) : (
          <div className='-mx-4 -my-4 overflow-x-auto sm:-mx-5 sm:-my-5'>
            <table className='w-full min-w-[720px] text-sm'>
              <thead>
                <tr className='border-b text-xs'>
                  <th className='text-muted-foreground px-3 py-1.5 text-left font-medium'>
                    {t('Channel')}
                  </th>
                  <th className={headerCellClassName}>{t('Requests')}</th>
                  <th className={headerCellClassName}>{t('Success')}</th>
                  <th className={headerCellClassName}>{t('Failed')}</th>
                  <th className={headerCellClassName}>{t('Input tokens')}</th>
                  <th className={headerCellClassName}>{t('Output tokens')}</th>
                  <th className={headerCellClassName}>{t('Cache Read')}</th>
                </tr>
              </thead>
              {channels.map((channel) => {
                const expanded = expandedChannels.has(channel.channel_id)
                const keysId = `channel-usage-keys-${channel.channel_id}`
                return (
                  <tbody key={channel.channel_id} id={keysId}>
                    <tr className='hover:bg-muted/40 border-b'>
                      <td className='px-3 py-1.5'>
                        <button
                          type='button'
                          onClick={() => toggleChannel(channel.channel_id)}
                          aria-expanded={expanded}
                          aria-controls={keysId}
                          className='flex w-full items-center gap-1.5 text-left'
                        >
                          <ChevronRight
                            aria-hidden='true'
                            className={cn(
                              'size-3.5 shrink-0 transition-transform',
                              expanded && 'rotate-90'
                            )}
                          />
                          <span className='truncate font-medium'>
                            {channel.channel_name || t('Deleted channel')}
                          </span>
                          <span className='text-muted-foreground shrink-0 text-xs'>
                            #{channel.channel_id}
                          </span>
                        </button>
                      </td>
                      <UsageCells totals={channel} locale={locale} />
                    </tr>
                    {expanded &&
                      channel.keys.map((key) => (
                        <tr
                          key={`${channel.channel_id}-${key.fingerprint || 'none'}`}
                          className='border-b last:border-0'
                        >
                          <td className='py-1.5 pr-3 pl-9'>
                            <div className='flex flex-col gap-0.5'>
                              <span className='truncate text-xs'>
                                {channelUsageKeyLabel(key, t)}
                              </span>
                              {key.models.length > 0 && (
                                <span className='text-muted-foreground truncate text-[11px]'>
                                  {key.models
                                    .map((model) => model.model_name)
                                    .join(' · ')}
                                </span>
                              )}
                            </div>
                          </td>
                          <UsageCells totals={key} locale={locale} />
                        </tr>
                      ))}
                  </tbody>
                )
              })}
            </table>
          </div>
        )}
      </PanelWrapper>
    </div>
  )
}

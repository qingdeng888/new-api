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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, Trash2, Power, PowerOff, Zap } from 'lucide-react'
import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { useProxyPools } from '@/features/system-settings/proxy-pool/hooks/use-proxy-pools'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { getServerMessage } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  getMultiKeyStatus,
  enableMultiKey,
  disableMultiKey,
  deleteMultiKey,
  enableAllMultiKeys,
  disableAllMultiKeys,
  deleteDisabledMultiKeys,
  setMultiKeyProxy,
} from '../../api'
import { MULTI_KEY_FILTER_OPTIONS } from '../../constants'
import {
  channelsQueryKeys,
  formatTimestamp,
  getMultiKeyStatusConfig,
  getMultiKeyConfirmMessage,
  handleTestChannel,
  isDestructiveAction,
  formatResponseTime,
  isOptionalProxyURL,
  parseProxyPoolBinding,
  buildProxyPoolBinding,
} from '../../lib'
import type { KeyStatus, MultiKeyConfirmAction } from '../../types'
import { useChannels } from '../channels-provider'
import { StatisticsCard } from './multi-key-statistics-card'
import { MultiKeyTableRowActions } from './multi-key-table-row-actions'

type KeyTestState = {
  status: 'testing' | 'success' | 'failed'
  time?: number
  message?: string
}

type MultiKeyManageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function MultiKeyManageDialog({
  open,
  onOpenChange,
}: MultiKeyManageDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((s) => s.auth.user)
  const { data: proxyPools = [] } = useProxyPools()
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  // Data state
  const [isLoading, setIsLoading] = useState(false)
  const [keys, setKeys] = useState<KeyStatus[]>([])
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [total, setTotal] = useState(0)
  const [totalPages, setTotalPages] = useState(0)
  const [enabledCount, setEnabledCount] = useState(0)
  const [manualDisabledCount, setManualDisabledCount] = useState(0)
  const [autoDisabledCount, setAutoDisabledCount] = useState(0)
  const [cooldownCount, setCooldownCount] = useState(0)

  // UI state
  const [statusFilter, setStatusFilter] = useState<number | null>(null)
  const [confirmAction, setConfirmAction] =
    useState<MultiKeyConfirmAction | null>(null)
  const [isPerformingAction, setIsPerformingAction] = useState(false)
  const [keyTests, setKeyTests] = useState<Record<number, KeyTestState>>({})
  const [batchTest, setBatchTest] = useState<{
    done: number
    total: number
  } | null>(null)
  const batchRunRef = useRef(0)
  const [proxyEditor, setProxyEditor] = useState<{
    index: number
    value: string
  } | null>(null)
  const [isSavingProxy, setIsSavingProxy] = useState(false)

  // Reset and load data when dialog opens
  useEffect(() => {
    if (open && currentRow) {
      setCurrentPage(1)
      setStatusFilter(null)
      setKeyTests({})
      setBatchTest(null)
      setProxyEditor(null)
      batchRunRef.current += 1
      loadKeyStatus(1, pageSize, null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, currentRow?.id])

  const loadKeyStatus = async (
    page: number = currentPage,
    size: number = pageSize,
    status: number | null = statusFilter
  ) => {
    if (!currentRow) return

    setIsLoading(true)
    try {
      const response = await getMultiKeyStatus(
        currentRow.id,
        page,
        size,
        status === null ? undefined : status
      )

      if (response.success && response.data) {
        setKeys(response.data.keys || [])
        setTotal(response.data.total || 0)
        setCurrentPage(response.data.page || 1)
        setPageSize(response.data.page_size || 10)
        setTotalPages(response.data.total_pages || 0)
        setEnabledCount(response.data.enabled_count || 0)
        setManualDisabledCount(response.data.manual_disabled_count || 0)
        setAutoDisabledCount(response.data.auto_disabled_count || 0)
        setCooldownCount(response.data.cooldown_count || 0)
      } else {
        handleServerError(response, t('Failed to load key status'))
      }
    } catch (error: unknown) {
      handleServerError(error, t('Failed to load key status'))
    } finally {
      setIsLoading(false)
    }
  }

  const handleStatusFilterChange = (value: string) => {
    const newFilter = value === 'all' ? null : Number.parseInt(value)
    setStatusFilter(newFilter)
    setCurrentPage(1)
    loadKeyStatus(1, pageSize, newFilter)
  }

  // Probe one key with the channel's first model; the result is shown per
  // row and never changes the key's status, matching manual channel tests.
  const runKeyTest = (keyIndex: number, silent: boolean): Promise<boolean> => {
    if (!currentRow) return Promise.resolve(false)
    const firstModel =
      currentRow.models
        ?.split(',')
        .map((model) => model.trim())
        .find(Boolean) || undefined
    setKeyTests((prev) => ({ ...prev, [keyIndex]: { status: 'testing' } }))
    return new Promise((resolve) => {
      void handleTestChannel(
        currentRow.id,
        {
          channelName: currentRow.name,
          testModel: firstModel,
          keyIndex,
          silent,
        },
        (success, responseTime, error) => {
          setKeyTests((prev) => ({
            ...prev,
            [keyIndex]: success
              ? { status: 'success', time: responseTime }
              : { status: 'failed', message: error || t('Test failed') },
          }))
          resolve(success)
        }
      )
    })
  }

  const handleTestKey = (keyIndex: number) => {
    if (!currentRow || keyTests[keyIndex]?.status === 'testing') return
    void runKeyTest(keyIndex, false)
  }

  // Test every key of the pool with bounded concurrency. Individual toasts
  // are suppressed; one summary toast reports the outcome.
  const handleTestAllKeys = async () => {
    if (!currentRow || batchTest) return
    const total = currentRow.channel_info?.multi_key_size || 0
    if (total <= 0) return
    const runId = ++batchRunRef.current
    setBatchTest({ done: 0, total })
    let cursor = 0
    let successCount = 0
    let done = 0
    const worker = async () => {
      while (cursor < total) {
        if (batchRunRef.current !== runId) return
        const keyIndex = cursor++
        const success = await runKeyTest(keyIndex, true)
        if (success) successCount += 1
        done += 1
        if (batchRunRef.current !== runId) return
        setBatchTest({ done, total })
      }
    }
    await Promise.all(Array.from({ length: Math.min(5, total) }, worker))
    if (batchRunRef.current !== runId) return
    setBatchTest(null)
    toast.success(
      t('Key tests finished: {{success}}/{{total}} succeeded', {
        success: successCount,
        total,
      })
    )
  }

  const editorMode = (() => {
    const parsed = parseProxyPoolBinding(proxyEditor?.value)
    if (!parsed) return 'manual'
    if (parsed.mode === 'random') return 'pool-random'
    if (parsed.mode === 'ordered') return 'pool-ordered'
    return 'pool-fixed'
  })()
  const proxyEditorInvalid =
    proxyEditor !== null &&
    editorMode === 'manual' &&
    proxyEditor.value.trim() !== '' &&
    proxyEditor.value.trim() !== 'direct' &&
    !isOptionalProxyURL(proxyEditor.value)

  const setEditorBinding = (value: string) =>
    setProxyEditor((prev) => (prev ? { ...prev, value } : prev))

  const handleEditorModeChange = (mode: string | null) => {
    if (mode === null) return
    if (mode === 'manual') {
      setEditorBinding('')
      return
    }
    const parsed = parseProxyPoolBinding(proxyEditor?.value)
    const poolId = parsed?.poolId ?? proxyPools[0]?.id ?? 0
    if (!poolId) {
      setEditorBinding('')
      return
    }
    if (mode === 'pool-fixed') {
      setEditorBinding(
        buildProxyPoolBinding(poolId, 'fixed', parsed?.fixedIndex ?? 0)
      )
      return
    }
    setEditorBinding(
      buildProxyPoolBinding(
        poolId,
        mode === 'pool-ordered' ? 'ordered' : 'random'
      )
    )
  }

  const describeProxyBinding = (value: string | undefined) => {
    if (!value) return null
    const parsed = parseProxyPoolBinding(value)
    if (parsed) {
      const poolName =
        proxyPools.find((pool) => pool.id === parsed.poolId)?.name ??
        t('Unknown pool')
      const modeLabel =
        parsed.mode === 'random'
          ? t('Random rotation')
          : parsed.mode === 'ordered'
            ? t('Ordered rotation')
            : t('Pinned pool proxy')
      return poolName + ' · ' + modeLabel
    }
    return value
  }

  // Save the per-key proxy: an empty value clears the override so the key
  // falls back to the channel-level proxy.
  const handleSaveProxy = async () => {
    if (!currentRow || !proxyEditor || proxyEditorInvalid) return
    setIsSavingProxy(true)
    try {
      const response = await setMultiKeyProxy(
        currentRow.id,
        proxyEditor.index,
        proxyEditor.value.trim()
      )
      if (response.success) {
        toast.success(getServerMessage(response) || t('Operation successful'))
        setProxyEditor(null)
        loadKeyStatus(currentPage, pageSize)
      } else {
        handleServerError(response, t('Operation failed'))
      }
    } catch (error: unknown) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setIsSavingProxy(false)
    }
  }

  const handlePageChange = (newPage: number) => {
    setCurrentPage(newPage)
    loadKeyStatus(newPage, pageSize)
  }

  const performAction = async () => {
    if (!confirmAction || !currentRow) return
    if (
      !canEditSensitive &&
      (confirmAction.type === 'delete' ||
        confirmAction.type === 'delete-disabled')
    ) {
      setConfirmAction(null)
      return
    }

    setIsPerformingAction(true)
    try {
      const { type, keyIndex } = confirmAction
      let response

      // Execute the appropriate action
      if (type === 'enable' && keyIndex !== undefined) {
        response = await enableMultiKey(currentRow.id, keyIndex)
      } else if (type === 'disable' && keyIndex !== undefined) {
        response = await disableMultiKey(currentRow.id, keyIndex)
      } else if (type === 'delete' && keyIndex !== undefined) {
        response = await deleteMultiKey(currentRow.id, keyIndex)
      } else if (type === 'enable-all') {
        response = await enableAllMultiKeys(currentRow.id)
      } else if (type === 'disable-all') {
        response = await disableAllMultiKeys(currentRow.id)
      } else if (type === 'delete-disabled') {
        response = await deleteDisabledMultiKeys(currentRow.id)
      }

      if (response?.success) {
        toast.success(getServerMessage(response) || t('Operation successful'))
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })

        // Reload data - reset to page 1 for bulk actions
        const isBulkAction = type.includes('all') || type === 'delete-disabled'
        if (isBulkAction) {
          setCurrentPage(1)
          loadKeyStatus(1, pageSize)
        } else {
          loadKeyStatus(currentPage, pageSize)
        }
      } else {
        handleServerError(response, t('Operation failed'))
      }
    } catch (error: unknown) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setIsPerformingAction(false)
      setConfirmAction(null)
    }
  }

  const renderStatusBadge = (key: KeyStatus) => {
    // A cooling key is still enabled, just temporarily out of rotation.
    if (
      key.status === 1 &&
      key.cooldown_until &&
      key.cooldown_until * 1000 > Date.now()
    ) {
      return (
        <span
          title={t('Cooling down until {{time}}', {
            time: formatTimestamp(key.cooldown_until),
          })}
        >
          <StatusBadge
            label={t('Cooldown')}
            variant='warning'
            showDot
            copyable={false}
          />
        </span>
      )
    }
    const config = getMultiKeyStatusConfig(key.status)
    return (
      <StatusBadge
        label={t(config.label)}
        variant={config.variant}
        showDot
        copyable={false}
      />
    )
  }

  const renderKeyTestResult = (keyIndex: number) => {
    const state = keyTests[keyIndex]
    if (!state) {
      return <span className='text-muted-foreground text-sm'>-</span>
    }
    if (state.status === 'testing') {
      return (
        <span
          className='text-muted-foreground flex items-center gap-1.5 text-sm'
          aria-label={t('Testing...')}
        >
          <Loader2 className='h-3.5 w-3.5 animate-spin' aria-hidden='true' />
          {t('Testing...')}
        </span>
      )
    }
    if (state.status === 'success') {
      return (
        <StatusBadge
          label={
            state.time !== undefined
              ? formatResponseTime(state.time, t)
              : t('Success')
          }
          variant='success'
          showDot={false}
          copyable={false}
        />
      )
    }
    const message = state.message || t('Test failed')
    return (
      <span
        title={message}
        className='text-destructive block max-w-[180px] truncate text-sm'
      >
        {message}
      </span>
    )
  }

  const formatKeyTimestamp = (timestamp?: number) => {
    if (!timestamp) return '-'
    return formatTimestamp(timestamp)
  }

  if (!currentRow) return null

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={
          <>
            {t('Multi-Key Management')}
            <StatusBadge
              label={currentRow.name}
              variant='neutral'
              copyable={false}
            />
            {currentRow.channel_info?.multi_key_mode && (
              <StatusBadge
                label={
                  currentRow.channel_info.multi_key_mode === 'random'
                    ? t('Random')
                    : t('Polling')
                }
                variant='neutral'
                copyable={false}
              />
            )}
          </>
        }
        description={t(
          'Manage multi-key status and configuration for this channel'
        )}
        contentClassName='flex max-h-[min(90dvh,var(--dialog-available-height))] max-w-5xl flex-col'
        titleClassName='flex items-center gap-2'
        contentHeight='min(72vh, 720px)'
        bodyClassName='space-y-4'
      >
        <div className='flex min-h-0 flex-1 flex-col space-y-4 overflow-hidden'>
          {/* Statistics */}
          <div className='grid shrink-0 grid-cols-2 gap-3 sm:grid-cols-4'>
            <StatisticsCard
              label={t('Enabled')}
              count={enabledCount}
              total={total}
            />
            <StatisticsCard
              label={t('Manual Disabled')}
              count={manualDisabledCount}
              total={total}
            />
            <StatisticsCard
              label={t('Auto Disabled')}
              count={autoDisabledCount}
              total={total}
            />
            <StatisticsCard
              label={t('Cooldown')}
              count={cooldownCount}
              total={total}
            />
          </div>

          <Separator className='shrink-0' />

          {/* Toolbar */}
          <div className='flex shrink-0 items-center justify-between'>
            <Select
              items={MULTI_KEY_FILTER_OPTIONS.map((option) => ({
                value: option.value,
                label: t(option.label),
              }))}
              value={statusFilter === null ? 'all' : statusFilter.toString()}
              onValueChange={(v) => v !== null && handleStatusFilterChange(v)}
            >
              <SelectTrigger className='w-40'>
                <SelectValue placeholder={t('All Status')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {MULTI_KEY_FILTER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.label)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>

            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => loadKeyStatus()}
                disabled={isLoading}
              >
                <RefreshCw className='h-4 w-4' />
              </Button>

              {(currentRow.channel_info?.multi_key_size || 0) > 0 && (
                <Button
                  variant='outline'
                  size='sm'
                  onClick={handleTestAllKeys}
                  disabled={Boolean(batchTest) || isLoading}
                >
                  {batchTest ? (
                    <Loader2 className='h-4 w-4 animate-spin' />
                  ) : (
                    <Zap className='h-4 w-4' />
                  )}
                  {batchTest
                    ? t('Testing {{done}}/{{total}}...', {
                        done: batchTest.done,
                        total: batchTest.total,
                      })
                    : t('Test All Keys')}
                </Button>
              )}

              {manualDisabledCount + autoDisabledCount > 0 && (
                <Button
                  variant='default'
                  size='sm'
                  onClick={() => setConfirmAction({ type: 'enable-all' })}
                >
                  <Power className='mr-2 h-4 w-4' />
                  {t('Enable All')}
                </Button>
              )}

              {enabledCount > 0 && (
                <Button
                  variant='destructive'
                  size='sm'
                  onClick={() => setConfirmAction({ type: 'disable-all' })}
                >
                  <PowerOff className='mr-2 h-4 w-4' />
                  {t('Disable All')}
                </Button>
              )}

              {autoDisabledCount > 0 && (
                <Button
                  variant='destructive'
                  size='sm'
                  onClick={() => {
                    if (!canEditSensitive) return
                    setConfirmAction({ type: 'delete-disabled' })
                  }}
                  disabled={!canEditSensitive}
                  title={
                    canEditSensitive
                      ? undefined
                      : t('No permission to perform this action')
                  }
                >
                  <Trash2 className='mr-2 h-4 w-4' />
                  {t('Delete Auto-Disabled')}
                </Button>
              )}
            </div>
          </div>
          {!canEditSensitive && (
            <p className='text-muted-foreground text-xs'>
              {t('No permission to perform this action')}
            </p>
          )}

          {/* Table */}
          <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
            {isLoading && (
              <div className='flex items-center justify-center py-12'>
                <Loader2 className='text-muted-foreground h-8 w-8 animate-spin' />
              </div>
            )}
            {!isLoading && keys.length === 0 && (
              <div className='text-muted-foreground py-12 text-center'>
                {t('No keys found')}
              </div>
            )}
            {!isLoading && !(keys.length === 0) && (
              <StaticDataTable
                className='rounded-none border-0'
                tableClassName='min-w-[1340px]'
                data={keys}
                getRowKey={(key) => key.index}
                columns={[
                  {
                    id: 'index',
                    header: t('Index'),
                    className: 'w-20',
                    cellClassName: 'font-mono text-sm',
                    cell: (key) => `#${key.index + 1}`,
                  },
                  {
                    id: 'key',
                    header: t('Key'),
                    className: 'min-w-[220px] max-w-[320px]',
                    cell: (key) => (
                      <div className='flex items-start gap-1.5'>
                        <span className='min-w-0 font-mono text-xs break-all'>
                          {key.key || '-'}
                        </span>
                        {key.key && (
                          <CopyButton
                            value={key.key}
                            className='text-muted-foreground hover:text-foreground size-6 shrink-0'
                            aria-label={t('Copy to clipboard')}
                          />
                        )}
                      </div>
                    ),
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    className: 'w-32',
                    cell: (key) => renderStatusBadge(key),
                  },
                  {
                    id: 'reason',
                    header: t('Disabled Reason'),
                    className: 'min-w-[200px]',
                    cellClassName: 'max-w-xs truncate text-sm',
                    cell: (key) => key.reason || '-',
                  },
                  {
                    id: 'disabled-time',
                    header: t('Disabled Time'),
                    className: 'w-44',
                    cellClassName: 'text-muted-foreground text-sm',
                    cell: (key) => formatKeyTimestamp(key.disabled_time),
                  },
                  {
                    id: 'proxy',
                    header: t('Proxy'),
                    className: 'min-w-[180px] max-w-[260px]',
                    cell: (key) => {
                      const label = describeProxyBinding(key.proxy)
                      return label ? (
                        <span
                          title={key.proxy}
                          className='block max-w-[240px] truncate font-mono text-xs'
                        >
                          {label}
                        </span>
                      ) : (
                        <span className='text-muted-foreground text-sm'>-</span>
                      )
                    },
                  },
                  {
                    id: 'test',
                    header: t('Test Result'),
                    className: 'w-44',
                    cellClassName: 'text-sm',
                    cell: (key) => renderKeyTestResult(key.index),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    className: 'text-right',
                    cell: (key) => (
                      <MultiKeyTableRowActions
                        keyIndex={key.index}
                        status={key.status}
                        canDelete={canEditSensitive}
                        isTesting={
                          Boolean(batchTest) ||
                          keyTests[key.index]?.status === 'testing'
                        }
                        onTest={() => handleTestKey(key.index)}
                        onSetProxy={() =>
                          setProxyEditor({
                            index: key.index,
                            value: key.proxy || '',
                          })
                        }
                        onAction={setConfirmAction}
                      />
                    ),
                  },
                ]}
              />
            )}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className='flex shrink-0 items-center justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Page {{current}} of {{total}}', {
                  current: currentPage,
                  total: totalPages,
                })}
              </div>
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(currentPage - 1)}
                  disabled={currentPage === 1 || isLoading}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => handlePageChange(currentPage + 1)}
                  disabled={currentPage >= totalPages || isLoading}
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          )}
        </div>
      </Dialog>

      {/* Confirmation Dialog */}
      <ConfirmDialog
        open={confirmAction !== null}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        title={t('Confirm Action')}
        desc={t(getMultiKeyConfirmMessage(confirmAction))}
        destructive={isDestructiveAction(confirmAction)}
        isLoading={isPerformingAction}
        handleConfirm={performAction}
      />

      {/* Per-key proxy editor */}
      <Dialog
        open={proxyEditor !== null}
        onOpenChange={(open) => !open && setProxyEditor(null)}
        title={t('Set proxy for key #{{index}}', {
          index: (proxyEditor?.index ?? 0) + 1,
        })}
        description={t(
          'Leave empty to use the channel proxy. Enter direct to dial without any proxy.'
        )}
        bodyClassName='space-y-3'
      >
        <Select
          value={editorMode}
          onValueChange={handleEditorModeChange}
          items={[
            { value: 'manual', label: t('Manual proxy address') },
            { value: 'pool-random', label: t('Random rotation') },
            { value: 'pool-ordered', label: t('Ordered rotation') },
            { value: 'pool-fixed', label: t('Pinned pool proxy') },
          ]}
        >
          <SelectTrigger className='w-full sm:w-56'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='manual'>
                {t('Manual proxy address')}
              </SelectItem>
              <SelectItem value='pool-random'>
                {t('Random rotation')}
              </SelectItem>
              <SelectItem value='pool-ordered'>
                {t('Ordered rotation')}
              </SelectItem>
              <SelectItem value='pool-fixed'>
                {t('Pinned pool proxy')}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        {editorMode === 'manual' && (
          <>
            <Input
              aria-label={t('Proxy Address')}
              placeholder='http://user:pass@ip:port  |  socks5://user:pass@ip:port'
              spellCheck={false}
              value={proxyEditor?.value ?? ''}
              onChange={(event) =>
                setProxyEditor((prev) =>
                  prev ? { ...prev, value: event.target.value } : prev
                )
              }
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !proxyEditorInvalid) {
                  event.preventDefault()
                  void handleSaveProxy()
                }
              }}
            />
            {proxyEditorInvalid && (
              <p className='text-destructive text-xs'>
                {t(
                  'Proxy address must use HTTP, HTTPS, SOCKS5, or SOCKS5H and include a valid host'
                )}
              </p>
            )}
          </>
        )}
        {editorMode !== 'manual' &&
          (proxyPools.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t(
                'No proxy pools configured yet. Create one in System Settings → Proxy pools first.'
              )}
            </p>
          ) : (
            <div className='flex flex-wrap gap-2'>
              <Select
                value={String(
                  parseProxyPoolBinding(proxyEditor?.value)?.poolId ??
                    proxyPools[0]?.id ??
                    0
                )}
                onValueChange={(next) => {
                  if (next === null) return
                  setEditorBinding(
                    buildProxyPoolBinding(
                      Number.parseInt(next, 10),
                      editorMode === 'pool-ordered'
                        ? 'ordered'
                        : editorMode === 'pool-fixed'
                          ? 'fixed'
                          : 'random',
                      parseProxyPoolBinding(proxyEditor?.value)?.fixedIndex
                    )
                  )
                }}
                items={proxyPools.map((pool) => ({
                  value: String(pool.id),
                  label: pool.name,
                }))}
              >
                <SelectTrigger className='w-full sm:w-56'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {proxyPools.map((pool) => (
                      <SelectItem key={pool.id} value={String(pool.id)}>
                        {pool.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {editorMode === 'pool-fixed' && (
                <Input
                  type='number'
                  min={0}
                  className='w-28'
                  value={
                    parseProxyPoolBinding(proxyEditor?.value)?.fixedIndex ?? 0
                  }
                  onChange={(event) => {
                    const poolId =
                      parseProxyPoolBinding(proxyEditor?.value)?.poolId ??
                      proxyPools[0]?.id ??
                      0
                    const memberIndex =
                      Number.parseInt(event.target.value, 10) || 0
                    setEditorBinding(
                      buildProxyPoolBinding(
                        poolId,
                        'fixed',
                        Math.max(0, memberIndex)
                      )
                    )
                  }}
                  aria-label={t('Fixed member index')}
                />
              )}
            </div>
          ))}
        <div className='flex justify-end gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() =>
              setProxyEditor((prev) => (prev ? { ...prev, value: '' } : prev))
            }
          >
            {t('Clear')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={() => setProxyEditor(null)}
          >
            {t('Cancel')}
          </Button>
          <Button
            size='sm'
            onClick={() => void handleSaveProxy()}
            disabled={proxyEditorInvalid || isSavingProxy}
          >
            {isSavingProxy && (
              <Loader2 className='h-4 w-4 animate-spin' aria-hidden='true' />
            )}
            {t('Save')}
          </Button>
        </div>
      </Dialog>
    </>
  )
}

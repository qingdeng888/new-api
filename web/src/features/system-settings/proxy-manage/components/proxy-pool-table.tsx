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
import {
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
  Upload,
  Zap,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { isOptionalProxyURL } from '@/features/channels/lib'
import { getServerMessage } from '@/lib/server-error-message'

import { testProxyPool } from '../api'
import { useProxyPool, useSaveProxyPool } from '../hooks/use-proxy-pool'
import type { ProxyPoolEntry, ProxyTestResult } from '../types'

type EditorState = {
  index: number | null
  url: string
  note: string
  enabled: boolean
}

const EMPTY_EDITOR: EditorState = {
  index: null,
  url: '',
  note: '',
  enabled: true,
}

type TestState = {
  status: 'testing' | 'done'
  result?: ProxyTestResult
}

export function ProxyPoolTable() {
  const { t } = useTranslation()
  const { data: entries = [], isLoading, refetch } = useProxyPool()
  const savePool = useSaveProxyPool()
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkText, setBulkText] = useState('')
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [tests, setTests] = useState<Record<string, TestState>>({})
  const [isRefreshing, setIsRefreshing] = useState(false)

  const editorInvalid = useMemo(() => {
    if (!editor) return false
    const url = editor.url.trim()
    if (!url) return true
    if (url === 'direct') return false
    return !isOptionalProxyURL(url)
  }, [editor])

  const selectedUrls = useMemo(
    () => entries.map((entry) => entry.url).filter((url) => selected.has(url)),
    [entries, selected]
  )
  const allSelected =
    entries.length > 0 && selectedUrls.length === entries.length
  const isTesting = Object.values(tests).some(
    (state) => state.status === 'testing'
  )

  const toggleSelection = (url: string, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (checked) next.add(url)
      else next.delete(url)
      return next
    })
  }

  const toggleAll = (checked: boolean) => {
    setSelected(
      checked ? new Set(entries.map((entry) => entry.url)) : new Set()
    )
  }

  // Re-reads the pool from the server so edits made outside this page (or by
  // another node) show up without a reload.
  const refreshEntries = async () => {
    setIsRefreshing(true)
    try {
      await refetch()
    } finally {
      setIsRefreshing(false)
    }
  }

  const persist = (next: ProxyPoolEntry[]) => {
    if (savePool.isPending) return
    savePool.mutate(next)
  }

  const handleSaveEntry = async () => {
    if (!editor || editorInvalid || savePool.isPending) return
    const entry: ProxyPoolEntry = {
      url: editor.url.trim(),
      note: editor.note.trim() || undefined,
      enabled: editor.enabled,
    }
    const next = [...entries]
    if (editor.index === null) {
      next.push(entry)
    } else {
      next[editor.index] = entry
    }
    // Two entries with the same URL would break row identity and selection.
    const duplicate = next.findIndex(
      (item, i) => i !== editor.index && item.url === entry.url
    )
    if (duplicate >= 0) {
      toast.error(
        t('This proxy already exists (entry {{index}})', {
          index: duplicate + 1,
        })
      )
      return
    }
    const saved = await savePool.mutateAsync(next).catch(() => null)
    // Keep the editor open with the draft when the backend rejects the entry.
    if (saved?.success) setEditor(null)
  }

  const handleToggle = (index: number, enabled: boolean) => {
    if (savePool.isPending) return
    const next = entries.map((entry, i) =>
      i === index ? { ...entry, enabled } : entry
    )
    persist(next)
  }

  const handleDelete = () => {
    if (deleteIndex === null || savePool.isPending) return
    const removed = entries[deleteIndex]
    persist(entries.filter((_, i) => i !== deleteIndex))
    if (removed) {
      setSelected((prev) => {
        const next = new Set(prev)
        next.delete(removed.url)
        return next
      })
      setTests((prev) => {
        const next = { ...prev }
        delete next[removed.url]
        return next
      })
    }
    setDeleteIndex(null)
  }

  const handleBulkAdd = () => {
    if (savePool.isPending) return
    const lines = bulkText
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line.length > 0 && !line.startsWith('#'))
    if (lines.length === 0) {
      setBulkOpen(false)
      return
    }
    const existing = new Set(entries.map((entry) => entry.url))
    const additions: ProxyPoolEntry[] = []
    let skipped = 0
    for (const line of lines) {
      if (
        existing.has(line) ||
        (line !== 'direct' && !isOptionalProxyURL(line))
      ) {
        skipped++
        continue
      }
      existing.add(line)
      additions.push({ url: line, enabled: true })
    }
    if (additions.length > 0) persist([...entries, ...additions])
    setBulkOpen(false)
    setBulkText('')
    if (skipped > 0) {
      toast.info(
        t('{{count}} entries skipped (duplicate or invalid)', {
          count: skipped,
        })
      )
    }
  }

  // Probes every requested proxy against a fixed public target; the backend
  // only accepts entries that belong to the pool.
  const runTests = async (urls: string[]) => {
    if (urls.length === 0) return
    setTests((prev) => {
      const next = { ...prev }
      for (const url of urls) next[url] = { status: 'testing' }
      return next
    })
    try {
      const response = await testProxyPool(urls)
      if (!response.success) {
        toast.error(getServerMessage(response) || t('Failed to test proxies'))
        setTests((prev) => {
          const next = { ...prev }
          for (const url of urls) delete next[url]
          return next
        })
        return
      }
      const results = response.data ?? []
      setTests((prev) => {
        const next = { ...prev }
        for (const result of results) {
          next[result.url] = { status: 'done', result }
        }
        return next
      })
      const failed = results.filter((result) => !result.ok).length
      if (failed === 0) {
        toast.success(
          t('All {{count}} proxies are reachable', { count: results.length })
        )
      } else {
        toast.warning(
          t('{{failed}} of {{count}} proxies failed', {
            failed,
            count: results.length,
          })
        )
      }
    } catch {
      toast.error(t('Failed to test proxies'))
      setTests((prev) => {
        const next = { ...prev }
        for (const url of urls) delete next[url]
        return next
      })
    }
  }

  const renderTestResult = (url: string) => {
    const state = tests[url]
    if (!state) {
      return <span className='text-muted-foreground text-sm'>-</span>
    }
    if (state.status === 'testing') {
      return (
        <span className='text-muted-foreground flex items-center gap-1.5 text-xs'>
          <Loader2 className='h-3.5 w-3.5 animate-spin' />
          {t('Testing...')}
        </span>
      )
    }
    const result = state.result
    if (!result) {
      return <span className='text-muted-foreground text-sm'>-</span>
    }
    if (result.ok) {
      return (
        <StatusBadge
          label={t('{{latency}}ms · HTTP {{status}}', {
            latency: result.latency_ms,
            status: result.status ?? '-',
          })}
          variant='success'
          showDot={false}
          copyable={false}
        />
      )
    }
    return (
      <span
        title={result.error}
        className='text-destructive block max-w-[220px] truncate text-xs'
      >
        {result.error || t('Unreachable')}
      </span>
    )
  }

  return (
    <>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Outbound proxy list for every channel and key. Keys bind to a specific entry or rotate randomly across all enabled entries; failing proxies cool down and transfer automatically.'
          )}
        </p>
        <div className='flex flex-wrap gap-2'>
          <Button size='sm' onClick={() => setEditor({ ...EMPTY_EDITOR })}>
            <Plus className='mr-1.5 h-4 w-4' />
            {t('Add proxy')}
          </Button>
          <Button variant='outline' size='sm' onClick={() => setBulkOpen(true)}>
            <Upload className='mr-1.5 h-4 w-4' />
            {t('Bulk add')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={isRefreshing || savePool.isPending}
            onClick={() => void refreshEntries()}
          >
            <RefreshCw
              className={`mr-1.5 h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`}
            />
            {t('Refresh')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={selectedUrls.length === 0 || isTesting}
            onClick={() => runTests(selectedUrls)}
          >
            {isTesting ? (
              <Loader2 className='mr-1.5 h-4 w-4 animate-spin' />
            ) : (
              <Zap className='mr-1.5 h-4 w-4' />
            )}
            {t('Test selected ({{count}})', { count: selectedUrls.length })}
          </Button>
        </div>
      </div>

      <StaticDataTable
        data={entries}
        getRowKey={(entry) => entry.url}
        emptyClassName='text-sm'
        emptyContent={t('No proxies configured yet.')}
        columns={[
          {
            id: 'select',
            header: (
              <Checkbox
                aria-label={t('Select all proxies')}
                checked={allSelected}
                indeterminate={selectedUrls.length > 0 && !allSelected}
                onCheckedChange={(checked) => toggleAll(checked === true)}
              />
            ),
            className: 'w-10',
            cell: (entry) => (
              <Checkbox
                aria-label={t('Select {{note}}', {
                  note: entry.note || entry.url,
                })}
                checked={selected.has(entry.url)}
                onCheckedChange={(checked) =>
                  toggleSelection(entry.url, checked === true)
                }
              />
            ),
          },
          {
            id: 'note',
            header: t('Note'),
            className: 'w-40',
            cellClassName: 'text-sm',
            cell: (entry) => entry.note || '-',
          },
          {
            id: 'url',
            header: t('Proxy address'),
            className: 'min-w-[240px]',
            cell: (entry) => (
              <span className='flex min-w-0 items-center gap-1.5'>
                <code className='bg-muted min-w-0 truncate rounded px-1.5 py-0.5 font-mono text-xs'>
                  {entry.url}
                </code>
                <CopyButton
                  value={entry.url}
                  size='icon'
                  className='size-6 shrink-0'
                  tooltip={t('Copy to clipboard')}
                  aria-label={t('Copy to clipboard')}
                />
              </span>
            ),
          },
          {
            id: 'status',
            header: t('Status'),
            className: 'w-24',
            cell: (entry) => (
              <StatusBadge
                label={entry.enabled ? t('Enabled') : t('Disabled')}
                variant={entry.enabled ? 'success' : 'neutral'}
                showDot
                copyable={false}
              />
            ),
          },
          {
            id: 'enable',
            header: t('Enable'),
            className: 'w-20',
            cell: (entry) => (
              <Switch
                checked={entry.enabled}
                aria-label={t('Enable')}
                onCheckedChange={(checked) =>
                  handleToggle(entries.indexOf(entry), checked)
                }
              />
            ),
          },
          {
            id: 'test',
            header: t('Test result'),
            className: 'min-w-[180px]',
            cell: (entry) => renderTestResult(entry.url),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'w-44 text-right',
            cell: (entry) => {
              const index = entries.indexOf(entry)
              const testing = tests[entry.url]?.status === 'testing'
              return (
                <div className='flex justify-end gap-1'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('Test this proxy')}
                    title={t('Test this proxy')}
                    disabled={testing}
                    onClick={() => void runTests([entry.url])}
                  >
                    {testing ? (
                      <Loader2 className='h-4 w-4 animate-spin' />
                    ) : (
                      <Zap className='h-4 w-4' />
                    )}
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('Edit')}
                    onClick={() =>
                      setEditor({
                        index,
                        url: entry.url,
                        note: entry.note ?? '',
                        enabled: entry.enabled,
                      })
                    }
                  >
                    <Pencil className='h-4 w-4' />
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('Delete')}
                    onClick={() => setDeleteIndex(index)}
                  >
                    <Trash2 className='h-4 w-4' />
                  </Button>
                </div>
              )
            },
          },
        ]}
      />

      {isLoading && (
        <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
      )}

      <Dialog
        open={editor !== null}
        onOpenChange={(open) => !open && setEditor(null)}
        title={
          editor?.index === null || editor?.index === undefined
            ? t('Add proxy')
            : t('Edit proxy')
        }
        description={t(
          'HTTP, HTTPS, SOCKS5 and SOCKS5H are supported, with optional user:password credentials. Enter direct to dial without a proxy.'
        )}
        bodyClassName='space-y-3'
        footer={
          <>
            <Button variant='outline' onClick={() => setEditor(null)}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleSaveEntry} disabled={editorInvalid}>
              {t('Save')}
            </Button>
          </>
        }
      >
        <div className='space-y-1.5'>
          <Label htmlFor='proxy-url'>{t('Proxy address')}</Label>
          <Input
            id='proxy-url'
            value={editor?.url ?? ''}
            spellCheck={false}
            placeholder='socks5://user:pass@ip:port'
            onChange={(event) =>
              setEditor((prev) =>
                prev ? { ...prev, url: event.target.value } : prev
              )
            }
          />
          {editorInvalid && (
            <p className='text-destructive text-xs'>
              {t(
                'Proxy address must use HTTP, HTTPS, SOCKS5, or SOCKS5H and include a valid host'
              )}
            </p>
          )}
        </div>
        <div className='space-y-1.5'>
          <Label htmlFor='proxy-note'>{t('Note')}</Label>
          <Input
            id='proxy-note'
            value={editor?.note ?? ''}
            placeholder={t('Optional, e.g. US datacenter')}
            onChange={(event) =>
              setEditor((prev) =>
                prev ? { ...prev, note: event.target.value } : prev
              )
            }
          />
        </div>
        <div className='flex items-center justify-between'>
          <Label htmlFor='proxy-enabled'>{t('Enable')}</Label>
          <Switch
            id='proxy-enabled'
            checked={editor?.enabled ?? true}
            onCheckedChange={(checked) =>
              setEditor((prev) => (prev ? { ...prev, enabled: checked } : prev))
            }
          />
        </div>
      </Dialog>

      <Dialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        title={t('Bulk add')}
        description={t(
          'One proxy per line. Blank lines and # comments are ignored; duplicates are skipped.'
        )}
        bodyClassName='space-y-2'
        footer={
          <>
            <Button variant='outline' onClick={() => setBulkOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleBulkAdd}>{t('Add')}</Button>
          </>
        }
      >
        <textarea
          value={bulkText}
          onChange={(event) => setBulkText(event.target.value)}
          rows={8}
          spellCheck={false}
          aria-label={t('Proxy address')}
          className='border-input bg-background focus-visible:border-ring focus-visible:ring-ring/50 flex min-h-40 w-full rounded-md border px-3 py-2 font-mono text-xs outline-none focus-visible:ring-3'
          placeholder={'http://user:pass@ip:port\nsocks5://ip:port\ndirect'}
        />
      </Dialog>

      <ConfirmDialog
        open={deleteIndex !== null}
        onOpenChange={(open) => !open && setDeleteIndex(null)}
        title={t('Delete proxy')}
        desc={t(
          'Delete this proxy? Keys pinned to it fall back to random rotation.'
        )}
        destructive
        isLoading={savePool.isPending}
        handleConfirm={handleDelete}
      />
    </>
  )
}

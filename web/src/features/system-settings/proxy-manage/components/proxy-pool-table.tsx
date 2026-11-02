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
import { Plus, RotateCcw, Trash2, Upload } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { isOptionalProxyURL } from '@/features/channels/lib'

import { useProxyPool, useSaveProxyPool } from '../hooks/use-proxy-pool'
import { maskProxyUrl, type ProxyPoolEntry } from '../types'

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

export function ProxyPoolTable() {
  const { t } = useTranslation()
  const { data: entries = [], isLoading } = useProxyPool()
  const savePool = useSaveProxyPool()
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkText, setBulkText] = useState('')
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)

  const editorInvalid = useMemo(() => {
    if (!editor) return false
    const url = editor.url.trim()
    if (!url) return true
    if (url === 'direct') return false
    return !isOptionalProxyURL(url)
  }, [editor])

  const persist = (next: ProxyPoolEntry[]) => savePool.mutate(next)

  const handleSaveEntry = () => {
    if (!editor || editorInvalid) return
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
    persist(next)
    setEditor(null)
  }

  const handleToggle = (index: number, enabled: boolean) => {
    const next = entries.map((entry, i) =>
      i === index ? { ...entry, enabled } : entry
    )
    persist(next)
  }

  const handleDelete = () => {
    if (deleteIndex === null) return
    persist(entries.filter((_, i) => i !== deleteIndex))
    setDeleteIndex(null)
  }

  const handleBulkAdd = () => {
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
    for (const line of lines) {
      if (
        existing.has(line) ||
        (line !== 'direct' && !isOptionalProxyURL(line))
      ) {
        continue
      }
      existing.add(line)
      additions.push({ url: line, enabled: true })
    }
    if (additions.length > 0) persist([...entries, ...additions])
    setBulkOpen(false)
    setBulkText('')
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
        </div>
      </div>

      <StaticDataTable
        data={entries}
        getRowKey={(entry) => entry.url}
        emptyClassName='text-sm'
        emptyContent={t('No proxies configured yet.')}
        columns={[
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
            className: 'min-w-[260px]',
            cell: (entry) => (
              <span className='flex min-w-0 items-center gap-1.5'>
                <code className='bg-muted min-w-0 truncate rounded px-1.5 py-0.5 font-mono text-xs'>
                  {maskProxyUrl(entry.url)}
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
            className: 'w-28',
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
            id: 'actions',
            header: t('Actions'),
            className: 'w-40 text-right',
            cell: (entry) => {
              const index = entries.indexOf(entry)
              return (
                <div className='flex justify-end gap-1'>
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
                    <RotateCcw className='h-4 w-4' />
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

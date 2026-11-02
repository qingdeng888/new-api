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
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'

import { useDeleteProxyPool } from '../hooks/use-proxy-pools'
import { parseProxyPoolEntries } from '../types'
import type { ProxyPool } from '../types'

type PoolTableProps = {
  pools: ProxyPool[]
  onEdit: (pool: ProxyPool) => void
  onCreate: () => void
}

export function PoolTable(props: PoolTableProps) {
  const { t } = useTranslation()
  const deletePool = useDeleteProxyPool()
  const [deleteTarget, setDeleteTarget] = useState<ProxyPool | null>(null)

  const handleDelete = async () => {
    if (!deleteTarget) return
    await deletePool.mutateAsync(deleteTarget.id)
    setDeleteTarget(null)
  }

  return (
    <>
      <div className='flex items-center justify-between'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Proxy pools are named proxy lists that channels and individual keys can bind to, with automatic failover on errors.'
          )}
        </p>
        <Button size='sm' onClick={props.onCreate}>
          <Plus className='mr-1.5 h-4 w-4' />
          {t('Create proxy pool')}
        </Button>
      </div>

      <StaticDataTable
        data={props.pools}
        getRowKey={(pool) => pool.id}
        emptyClassName='text-sm'
        emptyContent={t('No proxy pools configured yet.')}
        columns={[
          {
            id: 'name',
            header: t('Pool name'),
            cellClassName: 'font-medium text-sm',
            cell: (pool) => pool.name,
          },
          {
            id: 'count',
            header: t('Proxies'),
            className: 'w-24',
            cellClassName: 'text-sm',
            cell: (pool) => parseProxyPoolEntries(pool).length,
          },
          {
            id: 'preview',
            header: t('Preview'),
            className: 'min-w-[240px]',
            cellClassName: 'max-w-sm truncate font-mono text-xs',
            cell: (pool) => parseProxyPoolEntries(pool).join(', ') || '-',
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'w-28 text-right',
            cell: (pool) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Actions')}
                onEdit={() => props.onEdit(pool)}
                onDelete={() => setDeleteTarget(pool)}
              />
            ),
          },
        ]}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete proxy pool')}
        desc={t(
          'Delete proxy pool "{{name}}"? Bindings that reference it will stop resolving until they are re-pointed.',
          { name: deleteTarget?.name }
        )}
        destructive
        isLoading={deletePool.isPending}
        handleConfirm={handleDelete}
      />
    </>
  )
}

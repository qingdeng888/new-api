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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SettingsSection } from '../components/settings-section'
import { PoolFormDialog } from './components/pool-form-dialog'
import { PoolTable } from './components/pool-table'
import {
  useCreateProxyPool,
  useProxyPools,
  useUpdateProxyPool,
} from './hooks/use-proxy-pools'
import type { ProxyPool } from './types'

export function ProxyPoolSection() {
  const { t } = useTranslation()
  const { data: pools = [], isLoading } = useProxyPools()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingPool, setEditingPool] = useState<ProxyPool | null>(null)
  const createPool = useCreateProxyPool()
  const updatePool = useUpdateProxyPool()

  const handleCreate = () => {
    setEditingPool(null)
    setDialogOpen(true)
  }

  const handleEdit = (pool: ProxyPool) => {
    setEditingPool(pool)
    setDialogOpen(true)
  }

  const handleSubmit = (values: { name: string; proxies: string }) => {
    if (editingPool) {
      updatePool.mutate(
        { ...editingPool, name: values.name, proxies: values.proxies },
        {
          onSuccess: (res) => {
            if (res.success) setDialogOpen(false)
          },
        }
      )
    } else {
      createPool.mutate(values, {
        onSuccess: (res) => {
          if (res.success) setDialogOpen(false)
        },
      })
    }
  }

  return (
    <SettingsSection title={t('Proxy pools')}>
      {isLoading ? (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('Loading...')}
        </div>
      ) : (
        <PoolTable pools={pools} onEdit={handleEdit} onCreate={handleCreate} />
      )}

      <PoolFormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        pool={editingPool}
        saving={createPool.isPending || updatePool.isPending}
        onSubmit={handleSubmit}
      />
    </SettingsSection>
  )
}

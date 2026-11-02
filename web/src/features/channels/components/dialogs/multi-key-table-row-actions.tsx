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
import { Globe, Loader2, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { MultiKeyConfirmAction } from '../../types'

type MultiKeyTableRowActionsProps = {
  keyIndex: number
  status: number
  canDelete: boolean
  /** False for single-key channels: disabling/enabling/deleting the only
   * key is either a no-op or would leave the channel without keys. */
  isMultiKey: boolean
  isTesting?: boolean
  onTest?: () => void
  onSetProxy?: () => void
  onAction: (action: MultiKeyConfirmAction) => void
}

export function MultiKeyTableRowActions({
  keyIndex,
  status,
  canDelete,
  isMultiKey,
  isTesting = false,
  onTest,
  onSetProxy,
  onAction,
}: MultiKeyTableRowActionsProps) {
  const { t } = useTranslation()
  const isEnabled = status === 1

  return (
    <div className='flex justify-end gap-2'>
      {onSetProxy && (
        <Button
          variant='outline'
          size='sm'
          onClick={onSetProxy}
          aria-label={t('Set Proxy')}
        >
          <Globe className='h-4 w-4' aria-hidden='true' />
          {t('Proxy')}
        </Button>
      )}
      {onTest && (
        <Button
          variant='outline'
          size='sm'
          onClick={onTest}
          disabled={isTesting}
          aria-label={t('Test key')}
        >
          {isTesting ? (
            <Loader2 className='h-4 w-4 animate-spin' aria-hidden='true' />
          ) : (
            <Zap className='h-4 w-4' aria-hidden='true' />
          )}
          {t('Test')}
        </Button>
      )}
      {isMultiKey &&
        (isEnabled ? (
          <Button
            variant='outline'
            size='sm'
            onClick={() => onAction({ type: 'disable', keyIndex })}
          >
            {t('Disable')}
          </Button>
        ) : (
          <Button
            variant='outline'
            size='sm'
            onClick={() => onAction({ type: 'enable', keyIndex })}
          >
            {t('Enable')}
          </Button>
        ))}
      {isMultiKey && (
        <Button
          variant='destructive'
          size='sm'
          onClick={() => {
            if (!canDelete) return
            onAction({ type: 'delete', keyIndex })
          }}
          disabled={!canDelete}
          title={
            canDelete ? undefined : t('No permission to perform this action')
          }
        >
          {t('Delete')}
        </Button>
      )}
    </div>
  )
}

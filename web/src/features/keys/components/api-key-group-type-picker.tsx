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
import { Route, Tags } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import {
  AUTO_GROUP_TYPE_ID,
  type ApiKeyGroupTypeOption,
} from '../lib/api-key-group-types'

type ApiKeyGroupTypePickerProps = {
  options: ApiKeyGroupTypeOption[]
  autoAvailable: boolean
  value: string | null
  onValueChange: (typeID: string) => void
}

export function ApiKeyGroupTypePicker(props: ApiKeyGroupTypePickerProps) {
  const { t } = useTranslation()
  if (props.options.length === 0 && !props.autoAvailable) return null

  return (
    <div
      role='group'
      aria-label={t('Group type')}
      className='grid grid-cols-2 gap-2'
    >
      {props.autoAvailable && (
        <Button
          type='button'
          variant='outline'
          aria-pressed={props.value === AUTO_GROUP_TYPE_ID}
          data-group-type-card='auto'
          onClick={() => props.onValueChange(AUTO_GROUP_TYPE_ID)}
          className={cn(
            'h-auto min-h-16 w-full justify-start gap-3 px-3 py-2.5 text-left',
            props.value === AUTO_GROUP_TYPE_ID &&
              'border-primary/50 bg-primary/5'
          )}
        >
          <span className='bg-muted flex size-8 shrink-0 items-center justify-center rounded-full'>
            <Route className='size-4' aria-hidden='true' />
          </span>
          <span className='min-w-0'>
            <span className='block truncate text-sm font-medium'>
              {t('Automatic routing')}
            </span>
          </span>
        </Button>
      )}
      {props.options.map((option) => (
        <Button
          key={option.id}
          type='button'
          variant='outline'
          aria-pressed={props.value === option.id}
          data-group-type-card={option.id}
          onClick={() => props.onValueChange(option.id)}
          className={cn(
            'h-auto min-h-16 w-full justify-start gap-3 px-3 py-2.5 text-left',
            props.value === option.id && 'border-primary/50 bg-primary/5'
          )}
        >
          <span className='bg-muted flex size-8 shrink-0 items-center justify-center rounded-full'>
            {option.icon ? (
              getLobeIcon(option.icon, 22)
            ) : (
              <Tags className='size-4' aria-hidden='true' />
            )}
          </span>
          <span className='min-w-0'>
            <span className='block truncate text-sm font-medium'>
              {option.name || option.id}
            </span>
            <span className='text-muted-foreground block truncate text-xs'>
              {t('{{count}} available', { count: option.groups.length })}
            </span>
          </span>
        </Button>
      ))}
    </div>
  )
}

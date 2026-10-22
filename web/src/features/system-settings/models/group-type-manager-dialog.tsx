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
import { ArrowDown, ArrowUp, Plus, Tags, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { LobeIconField } from '@/components/lobe-icon-field'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { createGroupTypeId, type GroupTypeDefinition } from './group-type'

type GroupTypeManagerDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  types: GroupTypeDefinition[]
  groupCounts: Record<string, number>
  onApply: (types: GroupTypeDefinition[], removedTypeIDs: string[]) => void
}

function normalizedName(name: string): string {
  return name.trim().toLowerCase()
}

export function GroupTypeManagerDialog(props: GroupTypeManagerDialogProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<GroupTypeDefinition[]>(props.types)
  const [selectedTypeID, setSelectedTypeID] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] =
    useState<GroupTypeDefinition | null>(null)

  useEffect(() => {
    if (!props.open) return
    setDraft(props.types)
    setSelectedTypeID(props.types[0]?.id ?? null)
    setPendingDelete(null)
  }, [props.open, props.types])

  const selectedType = draft.find((type) => type.id === selectedTypeID) ?? null

  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>()
    for (const type of draft) {
      const name = normalizedName(type.name)
      if (!name) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
    return new Set(
      [...counts.entries()]
        .filter(([, count]) => count > 1)
        .map(([name]) => name)
    )
  }, [draft])

  const hasInvalidName = draft.some(
    (type) => !type.name.trim() || duplicateNames.has(normalizedName(type.name))
  )

  const updateType = (id: string, patch: Partial<GroupTypeDefinition>) => {
    setDraft((current) =>
      current.map((type) => (type.id === id ? { ...type, ...patch } : type))
    )
  }

  const addType = () => {
    const id = createGroupTypeId(new Set(draft.map((type) => type.id)))
    const nextType: GroupTypeDefinition = {
      id,
      name: t('New group type'),
      icon: '',
      order: draft.length,
    }
    setDraft((current) =>
      [...current, nextType].map((type, index) => ({
        ...type,
        order: index,
      }))
    )
    setSelectedTypeID(id)
  }

  const moveType = (index: number, direction: -1 | 1) => {
    const targetIndex = index + direction
    if (targetIndex < 0 || targetIndex >= draft.length) return
    setDraft((current) => {
      const next = [...current]
      ;[next[index], next[targetIndex]] = [next[targetIndex], next[index]]
      return next.map((type, order) => ({ ...type, order }))
    })
  }

  const removeType = (id: string) => {
    setDraft((current) => {
      const next = current.filter((type) => type.id !== id)
      return next.map((type, order) => ({ ...type, order }))
    })
    setSelectedTypeID((current) => {
      if (current !== id) return current
      return draft.find((type) => type.id !== id)?.id ?? null
    })
    setPendingDelete(null)
  }

  const handleDone = () => {
    if (hasInvalidName) return
    const appliedTypes = draft.map((type, index) => ({
      ...type,
      name: type.name.trim(),
      icon: type.icon.trim(),
      order: index,
    }))
    const remainingIDs = new Set(appliedTypes.map((type) => type.id))
    const removedTypeIDs = props.types
      .filter((type) => !remainingIDs.has(type.id))
      .map((type) => type.id)
    props.onApply(appliedTypes, removedTypeIDs)
    props.onOpenChange(false)
  }

  const selectedTypeGroupCount = selectedType
    ? (props.groupCounts[selectedType.id] ?? 0)
    : 0

  return (
    <>
      <Dialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('Group types')}
        description={t(
          'Create the categories shown before groups when users create API keys.'
        )}
        contentHeight='min(70vh, 620px)'
        contentClassName='sm:max-w-4xl'
        footer={
          <div className='flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <p className='text-muted-foreground text-xs'>
              {t('Changes are saved with group settings.')}
            </p>
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button onClick={handleDone} disabled={hasInvalidName}>
                {t('Done')}
              </Button>
            </div>
          </div>
        }
      >
        <div className='grid min-h-0 gap-4 md:grid-cols-[minmax(0,17rem)_minmax(0,1fr)]'>
          <div className='flex min-w-0 flex-col gap-3'>
            {draft.length === 0 && (
              <div className='border-border/70 bg-muted/30 rounded-lg border p-3 text-sm'>
                <p className='font-medium'>{t('No group types configured')}</p>
                <p className='text-muted-foreground mt-1 text-xs leading-5'>
                  {t(
                    'Adding the first group type enables type-first API key creation. User-selectable groups without a type will be hidden from that flow.'
                  )}
                </p>
              </div>
            )}

            <div className='flex flex-col gap-1.5'>
              {draft.map((type) => {
                const selected = type.id === selectedTypeID
                const count = props.groupCounts[type.id] ?? 0
                return (
                  <button
                    key={type.id}
                    type='button'
                    aria-pressed={selected}
                    onClick={() => setSelectedTypeID(type.id)}
                    className={cn(
                      'border-border/70 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors',
                      selected
                        ? 'border-primary/50 bg-primary/5'
                        : 'hover:bg-muted/50'
                    )}
                  >
                    <span className='flex size-6 shrink-0 items-center justify-center'>
                      {type.icon ? (
                        getLobeIcon(type.icon, 20)
                      ) : (
                        <Tags className='text-muted-foreground size-4' />
                      )}
                    </span>
                    <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                      {type.name.trim() || t('Untitled group type')}
                    </span>
                    <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                      {count}
                    </span>
                  </button>
                )
              })}
            </div>

            <Button
              variant='outline'
              className='justify-start'
              onClick={addType}
            >
              <Plus className='mr-2 size-4' />
              {t('Add group type')}
            </Button>
          </div>

          <div className='border-border/70 min-w-0 rounded-lg border p-4'>
            {!selectedType ? (
              <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
                {t('Select a group type to edit it.')}
              </div>
            ) : (
              <div className='flex min-w-0 flex-col gap-4'>
                <div className='flex flex-col gap-2'>
                  <Label htmlFor='group-type-name'>{t('Name')}</Label>
                  <Input
                    id='group-type-name'
                    value={selectedType.name}
                    maxLength={64}
                    aria-invalid={
                      !selectedType.name.trim() ||
                      duplicateNames.has(normalizedName(selectedType.name))
                    }
                    onChange={(event) =>
                      updateType(selectedType.id, { name: event.target.value })
                    }
                  />
                  {!selectedType.name.trim() && (
                    <p className='text-destructive text-xs'>
                      {t('Please enter a name')}
                    </p>
                  )}
                  {duplicateNames.has(normalizedName(selectedType.name)) && (
                    <p className='text-destructive text-xs'>
                      {t('Group type names must be unique.')}
                    </p>
                  )}
                </div>

                <div className='flex flex-col gap-2'>
                  <Label htmlFor='group-type-icon'>{t('Icon')}</Label>
                  <LobeIconField
                    id='group-type-icon'
                    value={selectedType.icon}
                    onChange={(icon) => updateType(selectedType.id, { icon })}
                  />
                </div>

                <div className='flex flex-wrap items-center gap-2'>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={draft[0]?.id === selectedType.id}
                    onClick={() => {
                      const index = draft.findIndex(
                        (type) => type.id === selectedType.id
                      )
                      moveType(index, -1)
                    }}
                  >
                    <ArrowUp className='mr-2 size-4' />
                    {t('Move up')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={draft.at(-1)?.id === selectedType.id}
                    onClick={() => {
                      const index = draft.findIndex(
                        (type) => type.id === selectedType.id
                      )
                      moveType(index, 1)
                    }}
                  >
                    <ArrowDown className='mr-2 size-4' />
                    {t('Move down')}
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-destructive hover:text-destructive ml-auto'
                    onClick={() => setPendingDelete(selectedType)}
                  >
                    <Trash2 className='mr-2 size-4' />
                    {t('Delete')}
                  </Button>
                </div>

                <p className='text-muted-foreground text-xs'>
                  {t('{{count}} group(s) use this type.', {
                    count: selectedTypeGroupCount,
                  })}
                </p>
              </div>
            )}
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('Delete group type')}
        desc={t(
          'Groups using this type will become unclassified. User-selectable groups among them will no longer appear when creating API keys.'
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={() => {
          if (pendingDelete) removeType(pendingDelete.id)
        }}
      />
    </>
  )
}

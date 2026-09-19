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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

import { getApiKeyModels } from '../../api'
import {
  buildCCSwitchURL,
  CCSWITCH_SOURCES,
  type CCSwitchApp,
} from '../../lib/cc-switch-sources'
import type { ApiKey } from '../../types'

type ModelOption = {
  value: string
  label: string
}

type ModelComboboxProps = {
  id?: string
  options: ModelOption[]
  value: string
  onValueChange: (value: string) => void
  placeholder?: string
  emptyText?: string
  disabled?: boolean
}

function ModelCombobox(props: ModelComboboxProps) {
  const labelMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const option of props.options) map.set(option.value, option.label)
    return map
  }, [props.options])

  return (
    <Combobox
      items={props.options.map((option) => option.value)}
      value={props.value || null}
      inputValue={props.value}
      onInputValueChange={props.onValueChange}
      onValueChange={(nextValue) => props.onValueChange(nextValue ?? '')}
      itemToStringLabel={(item) => labelMap.get(item) ?? item}
      itemToStringValue={(item) => item}
    >
      <ComboboxInput
        id={props.id}
        placeholder={props.placeholder}
        className='w-full'
        showClear={props.value.length > 0}
        disabled={props.disabled}
      />
      <ComboboxContent>
        <ComboboxList>
          <ComboboxCollection>
            {(item: string) => (
              <ComboboxItem key={item} value={item}>
                <span className='truncate'>{labelMap.get(item) ?? item}</span>
              </ComboboxItem>
            )}
          </ComboboxCollection>
        </ComboboxList>
        <ComboboxEmpty>{props.emptyText}</ComboboxEmpty>
      </ComboboxContent>
    </Combobox>
  )
}

function getServerAddress(): string {
  try {
    const raw = localStorage.getItem('status')
    if (raw) {
      const status = JSON.parse(raw) as { server_address?: unknown }
      if (typeof status.server_address === 'string' && status.server_address) {
        return status.server_address
      }
    }
  } catch {
    // Fall back to the browser origin when the cached status is unavailable.
  }
  return window.location.origin
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string
  apiKey?: Pick<ApiKey, 'id'> | null
}

export function CCSwitchDialog(props: Props) {
  const { t } = useTranslation()
  const [app, setApp] = useState<CCSwitchApp>('claude')
  const [name, setName] = useState<string>('')
  const [models, setModels] = useState<Record<string, string>>({})

  const {
    data: modelsData,
    isError: isModelsError,
    isFetching: isModelsFetching,
  } = useQuery({
    queryKey: ['cc-switch-models', props.apiKey?.id],
    queryFn: () => getApiKeyModels(props.tokenKey),
    enabled: props.open && !!props.apiKey?.id && !!props.tokenKey,
    staleTime: 0,
    refetchOnMount: 'always',
  })

  const canUseModels =
    !!props.apiKey?.id &&
    !!props.tokenKey &&
    !isModelsFetching &&
    !isModelsError

  const modelOptions = useMemo(() => {
    if (!canUseModels) return []
    const items = modelsData ?? []
    return [...new Set(items.map((model) => model.trim()).filter(Boolean))].map(
      (model) => ({ value: model, label: model })
    )
  }, [canUseModels, modelsData])

  useEffect(() => {
    if (!props.open) return
    setModels({})
    setApp('claude')
    setName('coder')
  }, [props.open])

  const currentSource =
    CCSWITCH_SOURCES.find((source) => source.appId === app) ??
    CCSWITCH_SOURCES[0]

  let modelEmptyText = t('No models found')
  if (isModelsFetching) {
    modelEmptyText = t('Loading...')
  } else if (isModelsError) {
    modelEmptyText = t('Failed to load')
  }

  const handleAppChange = (value: string) => {
    const source = CCSWITCH_SOURCES.find((item) => item.appId === value)
    if (!source) return
    setApp(source.appId)
    setName(source.defaultName)
    setModels({})
  }

  const handleSubmit = () => {
    if (!currentSource.supported || !canUseModels) return
    if (!models.model?.trim()) {
      toast.warning(t('Please select a primary model'))
      return
    }

    const key = props.tokenKey.startsWith('sk-')
      ? props.tokenKey
      : `sk-${props.tokenKey}`
    const url = buildCCSwitchURL(
      currentSource,
      name,
      models,
      key,
      getServerAddress()
    )
    if (!url) return
    window.open(url, '_blank')
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Import to CC Switch')}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      bodyClassName={
        currentSource.modelFields.length === 1 ? 'space-y-4 pb-52' : 'space-y-4'
      }
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!currentSource.supported || !canUseModels}
          >
            {currentSource.supported
              ? t('Open CC Switch')
              : t('CC Switch temporarily unavailable')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Application')}</Label>
          <RadioGroup
            value={app}
            onValueChange={handleAppChange}
            className='grid grid-cols-2 gap-2 sm:grid-cols-4'
          >
            {CCSWITCH_SOURCES.map((source) => (
              <div
                key={source.appId}
                className='flex min-w-0 items-center gap-2'
              >
                <RadioGroupItem
                  value={source.appId}
                  id={`app-${source.appId}`}
                />
                <Label
                  htmlFor={`app-${source.appId}`}
                  className='cursor-pointer truncate'
                >
                  {t(source.labelKey)}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        {!currentSource.supported ? (
          <p
            role='alert'
            className='rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-700 dark:text-amber-300'
          >
            {t(currentSource.unsupportedReasonKey ?? '')}
          </p>
        ) : (
          <div className='space-y-2'>
            <Label>{t('Name')}</Label>
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={currentSource.defaultName}
            />
          </div>
        )}

        {currentSource.modelFields.map((field) => (
          <div key={field.key} className='space-y-2'>
            <Label>
              {t(field.labelKey)}
              {field.required && (
                <span className='text-destructive ml-0.5'>*</span>
              )}
            </Label>
            <ModelCombobox
              options={modelOptions}
              value={models[field.key] ?? ''}
              onValueChange={(value) =>
                setModels((previous) => ({ ...previous, [field.key]: value }))
              }
              placeholder={t('Select or enter model name')}
              emptyText={modelEmptyText}
              disabled={!canUseModels}
            />
          </div>
        ))}
      </div>
    </Dialog>
  )
}

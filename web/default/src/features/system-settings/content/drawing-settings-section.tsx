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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const drawingSchema = z.object({
  DrawingEnabled: z.boolean(),
  MjNotifyEnabled: z.boolean(),
  MjAccountFilterEnabled: z.boolean(),
  MjForwardUrlEnabled: z.boolean(),
  MjModeClearEnabled: z.boolean(),
  MjActionCheckSuccessEnabled: z.boolean(),
  PlaygroundImageMaxConcurrency: z.number().int().min(0),
})

type DrawingFormValues = z.infer<typeof drawingSchema>
type DrawingSwitchName = Exclude<
  keyof DrawingFormValues,
  'PlaygroundImageMaxConcurrency'
>

type DrawingSettingsSectionProps = {
  defaultValues: DrawingFormValues
}

export function DrawingSettingsSection({
  defaultValues,
}: DrawingSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<DrawingFormValues>({
    resolver: zodResolver(drawingSchema),
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: DrawingFormValues) => {
    const updates = Object.entries(values).filter(
      ([key, value]) => value !== defaultValues[key as keyof DrawingFormValues]
    )

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value })
    }
  }

  const switches: Array<{
    name: DrawingSwitchName
    label: string
    description: string
  }> = [
    {
      name: 'DrawingEnabled',
      label: t('Enable drawing features'),
      description: t(
        'Required to expose MjProxy-style image generation to end users.'
      ),
    },
    {
      name: 'MjNotifyEnabled',
      label: t('Allow upstream callbacks'),
      description: t(
        'When enabled, MjProxy callbacks are accepted (reveals server IP).'
      ),
    },
    {
      name: 'MjAccountFilterEnabled',
      label: t('Allow accountFilter parameter'),
      description: t(
        'Keep enabled if you need to proxy requests for different upstream accounts.'
      ),
    },
    {
      name: 'MjForwardUrlEnabled',
      label: t('Rewrite callback URLs to the local server'),
      description: t(
        'Automatically replaces upstream callback URLs with the server address.'
      ),
    },
    {
      name: 'MjModeClearEnabled',
      label: t('Clear mode flags in prompts'),
      description: t(
        'Removes MjProxy flags such as --fast, --relax, and --turbo from user prompts.'
      ),
    },
    {
      name: 'MjActionCheckSuccessEnabled',
      label: t('Require job success before follow-up actions'),
      description: t(
        'Users must wait for a successful drawing before upscales or variations.'
      ),
    },
  ]

  return (
    <SettingsSection title={t('Drawing')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save drawing settings'
          />
          <div className='space-y-4'>
            {switches.map((item) => (
              <FormField
                key={item.name}
                control={form.control}
                name={item.name}
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{item.label}</FormLabel>
                      <FormDescription>{item.description}</FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />
            ))}
            <FormField
              control={form.control}
              name='PlaygroundImageMaxConcurrency'
              render={({ field }) => (
                <div className='flex min-w-0 flex-col gap-2 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4'>
                  <div className='min-w-0 space-y-0.5'>
                    <FormLabel>{t('Image playground concurrency')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Maximum number of image playground tasks running across the site. Use 0 for unlimited concurrency.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </div>
                  <FormControl>
                    <Input
                      className='w-full sm:w-32'
                      min={0}
                      step={1}
                      type='number'
                      value={field.value}
                      onBlur={field.onBlur}
                      onChange={(event) =>
                        field.onChange(
                          Math.max(
                            0,
                            Math.floor(Number(event.target.value) || 0)
                          )
                        )
                      }
                    />
                  </FormControl>
                </div>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

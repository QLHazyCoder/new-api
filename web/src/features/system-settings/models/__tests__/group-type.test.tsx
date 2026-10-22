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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import {
  getOptionValue,
  useSystemOptions,
} from '../../hooks/use-system-options'
import { parseGroupTypes, serializeGroupTypeMap } from '../group-type'
import { RatioSettingsCard } from '../ratio-settings-card'

const groupDefaults = {
  GroupRatio: '{"default":1,"vip":0.8}',
  TopupGroupRatio: '{}',
  UserUsableGroups: '{"default":"Standard","vip":"Premium"}',
  GroupDescriptions: '{}',
  GroupGroupRatio: '{}',
  AutoGroups: '[]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
  GroupTypes: '{}',
  GroupTypeMap: '{}',
}

const modelDefaults = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CompletionRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: '{}',
}

function SettingsFixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const { data } = useSystemOptions()
  const options = getOptionValue(data?.data, {
    ...groupDefaults,
    'group_ratio_setting.group_types': '{}',
    'group_ratio_setting.group_type_map': '{}',
  })

  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <RatioSettingsCard
        modelDefaults={modelDefaults}
        groupDefaults={{
          ...options,
          GroupTypes: options['group_ratio_setting.group_types'],
          GroupTypeMap: options['group_ratio_setting.group_type_map'],
        }}
        toolPricesDefault='{}'
        visibleTabs={['groups']}
      />
    </SettingsPageProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

describe('group type settings', () => {
  it('omits the reserved automatic routing ID from editable group types', () => {
    expect(
      parseGroupTypes(
        JSON.stringify({
          __auto__: { name: 'Imported routing' },
          vendor: { name: 'Vendor' },
        })
      )
    ).toEqual([{ id: 'vendor', name: 'Vendor', icon: '', order: 0 }])
  })

  it('drops mappings for deleted groups and the automatic routing entry', () => {
    const serialized = serializeGroupTypeMap(
      { paid: 'vendor', retired: 'vendor', auto: 'vendor' },
      new Set(['vendor']),
      new Set(['paid'])
    )

    expect(JSON.parse(serialized)).toEqual({ paid: 'vendor' })
  })

  it('rejects a reserved type ID in the production JSON form before saving', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: [] },
    })
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const rendered = render(
      <QueryClientProvider client={client}>
        <SettingsFixture />
      </QueryClientProvider>
    )
    try {
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
      const definitions = screen.getByRole('textbox', {
        name: 'Group type definitions',
      })
      fireEvent.input(definitions, {
        target: { value: '{"__auto__":{"name":"Imported routing","order":0}}' },
      })
      await user.click(
        screen.getByRole('button', { name: 'Save group settings' })
      )
      await waitFor(() =>
        expect(definitions).toHaveAttribute('aria-invalid', 'true')
      )
      expect(put).not.toHaveBeenCalled()
    } finally {
      rendered.unmount()
      client.clear()
    }
  })

  it('sends both maps in one request and preserves a failed draft across a settings refresh', async () => {
    const options = { ...groupDefaults }
    vi.spyOn(api, 'get').mockImplementation(async () => ({
      data: {
        success: true,
        data: Object.entries(options).map(([key, value]) => ({
          key,
          value: String(value),
        })),
      },
    }))
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValueOnce({
        data: { success: false, message: 'Database write failed' },
      })
      .mockResolvedValue({ data: { success: true, message: '' } })
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    client.setQueryData(['system-options'], {
      success: true,
      data: Object.entries(options).map(([key, value]) => ({
        key,
        value: String(value),
      })),
    })
    const rendered = render(
      <QueryClientProvider client={client}>
        <SettingsFixture />
      </QueryClientProvider>
    )
    try {
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
      const definitions = screen.getByRole('textbox', {
        name: 'Group type definitions',
      })
      const assignments = screen.getByRole('textbox', {
        name: 'Group type assignments',
      })
      const groupTypes = '{"vendor":{"name":"Vendor","order":0}}'
      const groupTypeMap = '{"default":"vendor"}'
      fireEvent.input(definitions, { target: { value: groupTypes } })
      fireEvent.input(assignments, { target: { value: groupTypeMap } })
      await user.click(
        screen.getByRole('button', { name: 'Save group settings' })
      )

      await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
      expect(put.mock.calls[0]).toEqual([
        '/api/option/group-types',
        { group_types: groupTypes, group_type_map: groupTypeMap },
      ])
      await waitFor(() =>
        expect(
          screen.getByRole('button', { name: 'Save group settings' })
        ).toBeEnabled()
      )
      // A separate option change causes the same props refresh as another settings tab.
      options.TopupGroupRatio = '{"vip":1.2}'
      await client.invalidateQueries({ queryKey: ['system-options'] })
      await waitFor(() =>
        expect(
          JSON.parse(
            (
              screen.getByRole('textbox', {
                name: 'Top-up group ratios',
              }) as HTMLTextAreaElement
            ).value
          )
        ).toEqual({ vip: 1.2 })
      )
      expect(definitions).toHaveValue(groupTypes)
      expect(assignments).toHaveValue(groupTypeMap)

      await user.click(
        screen.getByRole('button', { name: 'Save group settings' })
      )
      await waitFor(() => expect(put).toHaveBeenCalledTimes(2))
      expect(put.mock.calls[1]).toEqual(put.mock.calls[0])
    } finally {
      rendered.unmount()
      client.clear()
    }
  })
})

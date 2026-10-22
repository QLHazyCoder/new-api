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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { ApiKeysProvider } = await import('../api-keys-provider')
const { ApiKeysMutateDrawer } = await import('../api-keys-mutate-drawer')

type DrawerProps = Parameters<typeof ApiKeysMutateDrawer>[0]
type ApiKeyRow = NonNullable<DrawerProps['currentRow']>

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  post: ApiMethod
  put: ApiMethod
}
type RenderedDrawer = {
  queryClient: InstanceType<typeof QueryClient>
}

type FixtureGroupType = {
  id: string
  name: string
  icon?: string
  order?: number
}

type FixtureGroup = {
  desc: string
  ratio: number | string
  type?: FixtureGroupType
}

type DrawerFixture = {
  defaultUseAutoGroup: boolean
  groups: Record<string, FixtureGroup>
  groupTypes?: FixtureGroupType[]
  autoGroups: string[]
  maxAutoGroups: number
  existingApiKey?: ApiKeyRow
}

const DEFAULT_FIXTURE: DrawerFixture = {
  defaultUseAutoGroup: true,
  groups: {
    auto: { desc: 'Automatic routing', ratio: 'auto' },
    default: { desc: 'Standard access', ratio: 1 },
    vip: { desc: 'Priority access', ratio: 2 },
  },
  autoGroups: ['vip', 'default'],
  maxAutoGroups: 3,
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post
const originalPut = apiClient.put
let renderedDrawer: RenderedDrawer | null = null

function installApiFixtures(
  createdPayloads: Array<Record<string, unknown>>,
  fixture: DrawerFixture = DEFAULT_FIXTURE,
  updatedPayloads: Array<Record<string, unknown>> = []
) {
  apiClient.get = async (url) => {
    if (
      fixture.existingApiKey &&
      url === `/api/token/${fixture.existingApiKey.id}`
    ) {
      return { data: { success: true, data: fixture.existingApiKey } }
    }
    switch (url) {
      case '/api/status':
        return {
          data: {
            data: { default_use_auto_group: fixture.defaultUseAutoGroup },
          },
        }
      case '/api/user/models':
        return { data: { success: true, data: [] } }
      case '/api/user/self/groups':
        return {
          data: {
            success: true,
            data: fixture.groups,
            group_types: fixture.groupTypes,
          },
        }
      case '/api/token/auto-groups':
        return {
          data: {
            success: true,
            data: {
              groups: fixture.autoGroups,
              max_count: fixture.maxAutoGroups,
            },
          },
        }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  }
  apiClient.post = async (url, data) => {
    expect(url).toBe('/api/token/')
    expect(data && typeof data === 'object').toBeTruthy()
    createdPayloads.push(data as Record<string, unknown>)
    return { data: { success: true, data: {} } }
  }
  apiClient.put = async (url, data) => {
    expect(url).toBe('/api/token/')
    expect(data && typeof data === 'object').toBeTruthy()
    updatedPayloads.push(data as Record<string, unknown>)
    return { data: { success: true, data: {} } }
  }
}

async function renderCreateDrawer(
  fixture: DrawerFixture = DEFAULT_FIXTURE,
  options: { expectSaveEnabled?: boolean } = {}
): Promise<void> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(
    ['status'],
    { default_use_auto_group: fixture.defaultUseAutoGroup },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['user-models'],
    { success: true, data: [] },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['user-groups'],
    {
      success: true,
      data: fixture.groups,
      group_types: fixture.groupTypes,
    },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['token-auto-groups'],
    {
      success: true,
      data: {
        groups: fixture.autoGroups,
        max_count: fixture.maxAutoGroups,
      },
    },
    { updatedAt: freshAt }
  )
  renderedDrawer = { queryClient }

  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ApiKeysProvider>
          <ApiKeysMutateDrawer
            open
            onOpenChange={() => undefined}
            currentRow={fixture.existingApiKey}
          />
        </ApiKeysProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  await waitFor(
    () => {
      const saveButton = findButton('Save changes', false)
      expect(saveButton).not.toBeNull()
      if (options.expectSaveEnabled !== false) {
        expect(saveButton).toBeEnabled()
      }
    },
    { timeout: 1500 }
  )
}

function findButton(text: string, required: true): HTMLButtonElement
function findButton(text: string, required: false): HTMLButtonElement | null
function findButton(text: string, required = true): HTMLButtonElement | null {
  const button = screen
    .queryAllByRole<HTMLButtonElement>('button')
    .find((candidate) => candidate.textContent?.includes(text))
  if (required && !button) {
    throw new Error(`Expected button containing "${text}"`)
  }
  return button ?? null
}

function getControlByLabel(labelText: 'Name' | 'Quantity'): HTMLInputElement
function getControlByLabel(labelText: 'Group'): HTMLButtonElement
function getControlByLabel(labelText: 'Auto group order'): HTMLElement
function getControlByLabel(labelText: string): HTMLElement {
  const label = [...document.querySelectorAll<HTMLLabelElement>('label')].find(
    (candidate) => candidate.textContent?.trim() === labelText
  )
  if (!label) {
    throw new Error(`Expected label "${labelText}"`)
  }

  const control =
    label.control ??
    label
      .closest('[data-slot="form-item"]')
      ?.querySelector<HTMLElement>(
        '[data-slot="form-control"], input, textarea, button[role="combobox"], [role="group"]'
      )
  if (!control) {
    throw new Error(`Expected control for label "${labelText}"`)
  }
  return control
}

function changeInput(input: HTMLInputElement, value: string): void {
  fireEvent.input(input, { target: { value } })
}

function selectComboboxOption(
  trigger: HTMLButtonElement,
  optionDescription: string
): void {
  fireEvent.click(trigger)
  const option = [
    ...document.querySelectorAll<HTMLElement>('[data-slot="command-item"]'),
  ].find((candidate) => candidate.textContent?.includes(optionDescription))
  if (!option) {
    throw new Error(`Expected option containing "${optionDescription}"`)
  }
  fireEvent.click(option)
}

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
  apiClient.put = originalPut
  localStorage.clear()
  if (renderedDrawer) {
    renderedDrawer.queryClient.clear()
    renderedDrawer = null
  }
})

describe('API keys mutate drawer Auto group integration', () => {
  test('inherits the root Auto order and sends an empty override for every batch-created key', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    const groupTrigger = getControlByLabel('Group')
    expect(groupTrigger.textContent?.includes('auto')).toBe(true)
    expect(
      document.body.textContent?.includes(
        'Using the complete global Auto order (2 groups)'
      )
    ).toBe(true)
    expect(
      [
        ...document.querySelectorAll('[data-slot="global-auto-order-name"]'),
      ].map((item) => item.textContent)
    ).toEqual(['vip', 'default'])
    expect(findButton('Restore global Auto', true).disabled).toBe(true)

    changeInput(getControlByLabel('Name'), 'batch')
    changeInput(getControlByLabel('Quantity'), '2')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(2))

    expect(createdPayloads.length).toBe(2)
    expect(createdPayloads[0]?.name).toBe('batch')
    for (const payload of createdPayloads) {
      expect(payload.group).toBe('auto')
      expect(payload.auto_groups).toEqual([])
      expect(payload.cross_group_retry).toBe(true)
    }
  })

  test('preserves an unsaved custom order and mode after Auto to ordinary to Auto changes', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    const autoOrderControl = getControlByLabel('Auto group order')
    const addGroupTrigger = autoOrderControl.querySelector<HTMLButtonElement>(
      'button[role="combobox"]'
    )
    if (!addGroupTrigger) {
      throw new Error('Expected Auto group order combobox')
    }
    selectComboboxOption(addGroupTrigger, 'Priority access')

    expect(
      document.querySelector('button[aria-label="Remove vip"]')
    ).toBeTruthy()
    expect(document.body.textContent?.includes('1 / 3 groups selected')).toBe(
      true
    )
    expect(findButton('Restore global Auto', true).disabled).toBe(false)

    const groupTrigger = getControlByLabel('Group')
    selectComboboxOption(groupTrigger, 'Standard access')
    expect(document.querySelector('button[aria-label="Remove vip"]')).toBe(null)
    selectComboboxOption(groupTrigger, 'Automatic routing')

    expect(
      document.querySelector('button[aria-label="Remove vip"]')
    ).toBeTruthy()
    expect(document.body.textContent?.includes('1 / 3 groups selected')).toBe(
      true
    )
    expect(findButton('Restore global Auto', true).disabled).toBe(false)

    changeInput(getControlByLabel('Name'), 'custom')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.auto_groups).toEqual(['vip'])
  })
})

describe('API keys mutate drawer user quota query switch', () => {
  test('sends false for a new key after the switch, which starts on, is turned off', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    fireEvent.click(findButton('Advanced Settings', true))
    const quotaQuerySwitch = await screen.findByRole('switch', {
      name: 'Allow querying user quota',
    })
    expect(quotaQuerySwitch).toBeChecked()
    fireEvent.click(quotaQuerySwitch)
    expect(quotaQuerySwitch).not.toBeChecked()

    changeInput(getControlByLabel('Name'), 'no-user-quota')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.allow_user_quota_query).toBe(false)
  })
})

const TYPE_A: FixtureGroupType = {
  id: 'type-a',
  name: 'Type A',
  icon: 'OpenAI',
  order: 1,
}
const TYPE_B: FixtureGroupType = {
  id: 'type-b',
  name: 'Type B',
  icon: 'Claude',
  order: 2,
}
const TYPED_FIXTURE: DrawerFixture = {
  defaultUseAutoGroup: false,
  groups: {
    auto: { desc: 'Automatic routing', ratio: 'auto' },
    default: { desc: 'Standard access', ratio: 1, type: TYPE_A },
    vip: { desc: 'Priority access', ratio: 2, type: TYPE_B },
    untyped: { desc: 'Untyped access', ratio: 1 },
  },
  groupTypes: [TYPE_A, TYPE_B],
  autoGroups: ['vip', 'default'],
  maxAutoGroups: 3,
}

describe('API keys mutate drawer group types', () => {
  test('requires a type before selecting a group and filters groups by type', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads, TYPED_FIXTURE)
    await renderCreateDrawer(TYPED_FIXTURE)

    const typeA = screen.getByRole('button', { name: /Type A/ })
    const typeB = screen.getByRole('button', { name: /Type B/ })
    expect(typeA).toHaveAttribute('aria-pressed', 'false')
    expect(typeB).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByText('Untyped access')).toBe(null)

    const groupTrigger = getControlByLabel('Group')
    expect(groupTrigger.disabled).toBe(true)

    fireEvent.click(typeA)
    expect(typeA).toHaveAttribute('aria-pressed', 'true')
    expect(groupTrigger.disabled).toBe(false)
    expect(groupTrigger.textContent).toContain('default')

    fireEvent.click(groupTrigger)
    const options = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="command-item"]'),
    ]
    expect(options).toHaveLength(1)
    expect(options[0].textContent).toContain('Standard access')
    fireEvent.click(options[0])

    changeInput(getControlByLabel('Name'), 'typed')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.group).toBe('default')
    expect(createdPayloads[0]).not.toHaveProperty('type')
  })

  test('selects the built-in automatic routing card', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads, TYPED_FIXTURE)
    await renderCreateDrawer(TYPED_FIXTURE)

    fireEvent.click(screen.getByRole('button', { name: 'Automatic routing' }))
    const groupTrigger = getControlByLabel('Group')
    expect(groupTrigger.textContent).toContain('auto')
    expect(document.body.textContent).toContain(
      'Using the complete global Auto order (2 groups)'
    )

    changeInput(getControlByLabel('Name'), 'typed-auto')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.group).toBe('auto')
    expect(createdPayloads[0]?.cross_group_retry).toBe(true)
  })

  test('shows an empty state and blocks saving when no typed groups are available', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    const fixture: DrawerFixture = {
      defaultUseAutoGroup: false,
      groups: {
        untyped: { desc: 'Untyped access', ratio: 1 },
      },
      groupTypes: [TYPE_A],
      autoGroups: [],
      maxAutoGroups: 3,
    }
    installApiFixtures(createdPayloads, fixture)
    await renderCreateDrawer(fixture, { expectSaveEnabled: false })

    expect(
      screen.getByText(
        'No groups are available under the configured group types. Please contact the administrator.'
      )
    ).toBeVisible()
    expect(getControlByLabel('Group').disabled).toBe(true)
    expect(findButton('Save changes', true).disabled).toBe(true)
    expect(screen.queryByText('Untyped access')).toBe(null)
  })

  test('ignores the reserved automatic routing ID returned as a type', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    const reservedType: FixtureGroupType = {
      id: '__auto__',
      name: 'Imported routing',
      order: 0,
    }
    const fixture: DrawerFixture = {
      defaultUseAutoGroup: false,
      groups: {
        auto: { desc: 'Automatic routing', ratio: 'auto' },
        default: { desc: 'Standard access', ratio: 1 },
      },
      groupTypes: [reservedType],
      autoGroups: ['default'],
      maxAutoGroups: 3,
    }
    installApiFixtures(createdPayloads, fixture)
    await renderCreateDrawer(fixture)

    expect(screen.queryByRole('button', { name: /Imported routing/ })).toBe(
      null
    )
    expect(getControlByLabel('Group').disabled).toBe(false)
  })
})

const EXISTING_UNCLASSIFIED_KEY: ApiKeyRow = {
  id: 7,
  name: 'legacy',
  key: 'sk-legacy',
  status: 1,
  remain_quota: 0,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'untyped',
  auto_groups: null,
  cross_group_retry: false,
  model_limits_enabled: false,
  model_limits: '',
  allow_ips: '',
  allow_user_quota_query: false,
}

describe('API keys mutate drawer legacy unclassified groups', () => {
  test('warns but preserves the existing group when no type is selected', async () => {
    const updatedPayloads: Array<Record<string, unknown>> = []
    const fixture: DrawerFixture = {
      ...TYPED_FIXTURE,
      existingApiKey: EXISTING_UNCLASSIFIED_KEY,
    }
    installApiFixtures([], fixture, updatedPayloads)
    await renderCreateDrawer(fixture)

    expect(
      screen.getByText(
        'This key currently uses a group without a type. It will keep working, but select a type to change the group.'
      )
    ).toBeVisible()
    const groupTrigger = getControlByLabel('Group')
    expect(groupTrigger.disabled).toBe(true)
    expect(groupTrigger.textContent).toContain('untyped')

    changeInput(getControlByLabel('Name'), 'legacy-renamed')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(updatedPayloads).toHaveLength(1))
    expect(updatedPayloads[0]?.id).toBe(7)
    expect(updatedPayloads[0]?.group).toBe('untyped')
  })
})

const EXISTING_EMPTY_GROUP_KEY: ApiKeyRow = {
  ...EXISTING_UNCLASSIFIED_KEY,
  id: 8,
  group: '',
}

describe('API keys mutate drawer legacy empty groups', () => {
  test('allows saving when no typed groups are available', async () => {
    const updatedPayloads: Array<Record<string, unknown>> = []
    const fixture: DrawerFixture = {
      defaultUseAutoGroup: false,
      groups: {
        untyped: { desc: 'Untyped access', ratio: 1 },
      },
      groupTypes: [TYPE_A],
      autoGroups: [],
      maxAutoGroups: 3,
      existingApiKey: EXISTING_EMPTY_GROUP_KEY,
    }
    installApiFixtures([], fixture, updatedPayloads)
    await renderCreateDrawer(fixture)

    expect(
      screen.getByText(
        'No groups are available under the configured group types. Please contact the administrator.'
      )
    ).toBeVisible()
    expect(findButton('Save changes', true).disabled).toBe(false)

    changeInput(getControlByLabel('Name'), 'legacy-empty')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(updatedPayloads).toHaveLength(1))
    expect(updatedPayloads[0]?.id).toBe(8)
    expect(updatedPayloads[0]?.group).toBe('')
  })
})

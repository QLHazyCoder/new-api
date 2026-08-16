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
import assert from 'node:assert/strict'

import { Window } from 'happy-dom'

const bunTestModule = 'bun:test'
const { afterAll, describe, test } = (await import(bunTestModule)) as {
  afterAll: typeof import('node:test').after
  describe: typeof import('node:test').describe
  test: typeof import('node:test').test
}

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'NodeFilter',
  'Element',
  'Event',
  'KeyboardEvent',
  'PointerEvent',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { AmountDiscountVisualEditor } =
  await import('../amount-discount-visual-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function findButton(container: ParentNode, label: string) {
  const button = [
    ...container.querySelectorAll<HTMLButtonElement>('button'),
  ].find((candidate) => candidate.textContent?.includes(label))
  assert.ok(button)
  return button
}

function Harness(props: {
  initialGroups?: string[]
  availableGroups?: string[]
  groupsError?: boolean
  onRetryGroups?: () => void
}) {
  const [groups, setGroups] = useState(props.initialGroups ?? [])
  return (
    <I18nextProvider i18n={i18n}>
      <AmountDiscountVisualEditor
        value='{}'
        onChange={() => undefined}
        eligibleGroups={groups}
        availableGroups={props.availableGroups ?? []}
        onEligibleGroupsChange={setGroups}
        groupsError={props.groupsError}
        onRetryGroups={props.onRetryGroups}
      />
      <output data-testid='eligible-groups'>{groups.join(',')}</output>
    </I18nextProvider>
  )
}

async function renderHarness(props: React.ComponentProps<typeof Harness>) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => root.render(<Harness {...props} />))
  return { container, root }
}

async function unmountHarness(
  rendered: Awaited<ReturnType<typeof renderHarness>>
) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

describe('AmountDiscountVisualEditor group whitelist', () => {
  afterAll(() => {
    domWindow.close()
  })

  test('shows the empty state and selects a group from an upward portal', async () => {
    const rendered = await renderHarness({
      availableGroups: ['default', 'vip'],
    })

    assert.match(
      rendered.container.textContent ?? '',
      /Available groups:\s*Not set/
    )

    await act(async () =>
      findButton(rendered.container, 'Modify available groups').click()
    )
    const input = rendered.container.querySelector<HTMLInputElement>(
      '#amount-discount-eligible-groups'
    )
    assert.ok(input)

    await act(async () => {
      input.focus()
      input.click()
    })

    const popup = document.querySelector<HTMLElement>(
      '[data-slot="combobox-content"]'
    )
    assert.ok(popup)
    assert.equal(popup.dataset.side, 'top')

    const vipItem = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="combobox-item"]'),
    ].find((item) => item.textContent?.includes('vip'))
    assert.ok(vipItem)
    await act(async () => vipItem.click())
    assert.equal(
      rendered.container.querySelector('[data-testid="eligible-groups"]')
        ?.textContent,
      'vip'
    )

    await act(async () =>
      findButton(rendered.container, 'Finish editing available groups').click()
    )
    assert.match(
      rendered.container.textContent ?? '',
      /Available groups:\s*vip/
    )

    await unmountHarness(rendered)
  })

  test('keeps a previously selected group that is no longer occupied', async () => {
    const rendered = await renderHarness({
      initialGroups: ['legacy'],
      availableGroups: ['default'],
    })

    await act(async () =>
      findButton(rendered.container, 'Modify available groups').click()
    )
    assert.match(rendered.container.textContent ?? '', /legacy/)

    await unmountHarness(rendered)
  })

  test('disables editing on load failure and preserves the retry action', async () => {
    let retries = 0
    const rendered = await renderHarness({
      groupsError: true,
      onRetryGroups: () => {
        retries += 1
      },
    })

    assert.equal(
      findButton(rendered.container, 'Modify available groups').disabled,
      true
    )
    await act(async () => findButton(rendered.container, 'Retry').click())
    assert.equal(retries, 1)

    await unmountHarness(rendered)
  })
})

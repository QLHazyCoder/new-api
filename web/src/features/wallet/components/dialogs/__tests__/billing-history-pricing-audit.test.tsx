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
  'Node',
  'Element',
  'Event',
  'CustomEvent',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { TopUpPricingAudit } = await import('../billing-history-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

async function renderAudit(
  props: React.ComponentProps<typeof TopUpPricingAudit>
) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <TopUpPricingAudit {...props} />
      </I18nextProvider>
    )
  })
  return { container, root }
}

describe('TopUpPricingAudit', () => {
  afterAll(() => {
    domWindow.close()
  })

  test('renders the immutable pricing inputs and formula', async () => {
    const rendered = await renderAudit({
      showLegacy: false,
      snapshot: {
        version: 1,
        payment_provider: 'epay',
        user_group: 'vip',
        quota_display_type: 'USD',
        requested_amount: 200,
        normalized_amount: 200,
        stored_amount: 200,
        quota_per_unit: 500000,
        unit_price: 1.5,
        topup_group_ratio: 2,
        amount_discount_eligible: true,
        amount_discount_applied: true,
        amount_discount_rate: 0.9,
        pay_money: 540,
      },
    })

    const text = rendered.container.textContent ?? ''
    assert.match(text, /v1 \/ epay \/ vip \/ USD/)
    assert.match(text, /200 x 1\.5 x 2 x 0\.9 = 540/)
    assert.match(text, /Amount discount:\s*Applied/)

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })

  test('distinguishes legacy records and excluded gateways', async () => {
    const legacy = await renderAudit({ showLegacy: true })
    assert.match(
      legacy.container.textContent ?? '',
      /Legacy order, no pricing snapshot/
    )
    await act(async () => legacy.root.unmount())
    legacy.container.remove()

    const excluded = await renderAudit({ showLegacy: false })
    assert.equal(excluded.container.textContent, '')
    await act(async () => excluded.root.unmount())
    excluded.container.remove()
  })
})

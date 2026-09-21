import { afterEach, describe, expect, test } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { displayAmountToQuotaExact } from '../currency'

describe('displayAmountToQuotaExact', () => {
  afterEach(() => {
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  test('preserves token values above the JavaScript safe integer limit', () => {
    useSystemConfigStore.setState({
      config: {
        ...useSystemConfigStore.getState().config,
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      },
    })

    expect(displayAmountToQuotaExact('9007199254740993')).toBe(
      9007199254740993n
    )
  })

  test('converts currency display values with exact decimal arithmetic', () => {
    useSystemConfigStore.setState({
      config: {
        ...useSystemConfigStore.getState().config,
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'USD',
          quotaPerUnit: 500000,
        },
      },
    })

    expect(displayAmountToQuotaExact('1.000001')).toBe(500001n)
  })
})

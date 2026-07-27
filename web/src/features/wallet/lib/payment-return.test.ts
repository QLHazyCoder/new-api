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
import { describe, test } from 'node:test'

import type { UserWalletData } from '../types'
import {
  hasQuotaChanged,
  hasRecentPaymentMarker,
  parsePaymentReturnState,
} from './payment-return'

describe('payment return state', () => {
  test('accepts supported return parameters and rejects unrelated searches', () => {
    assert.deepEqual(
      parsePaymentReturnState({
        show_history: 'true',
        pay: 'pending',
        scope: 'subscription',
      }),
      {
        showHistory: true,
        pay: 'pending',
        scope: 'subscription',
      }
    )
    assert.equal(parsePaymentReturnState({ page: 'wallet' }), null)
  })

  test('limits marker polling to the configured age window', () => {
    const marker = {
      scope: 'topup' as const,
      source: 'new_tab' as const,
      createdAt: 1_000,
    }

    assert.equal(hasRecentPaymentMarker(marker, 1_000), true)
    assert.equal(hasRecentPaymentMarker(marker, 601_000), true)
    assert.equal(hasRecentPaymentMarker(marker, 601_001), false)
    assert.equal(hasRecentPaymentMarker(marker, 999), false)
  })

  test('detects quota changes without treating missing snapshots as success', () => {
    const previous: UserWalletData = {
      id: 1,
      username: 'wallet-test',
      quota: 100,
      used_quota: 20,
      request_count: 3,
      aff_quota: 5,
      aff_history_quota: 10,
      aff_count: 2,
      group: 'default',
    }

    assert.equal(hasQuotaChanged(previous, { ...previous }), false)
    assert.equal(
      hasQuotaChanged(previous, { ...previous, used_quota: 21 }),
      true
    )
    assert.equal(hasQuotaChanged(previous, null), false)
  })
})

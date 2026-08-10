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

import { getLogStatusDisplay } from '../format'

const consumeType = { label: 'Consume', color: 'green' }

describe('request outcome status display', () => {
  test('renders a new failed consume outcome as Failed', () => {
    assert.deepEqual(
      getLogStatusDisplay(consumeType, {
        request_outcome: {
          status: 'failed',
          reason: 'no_billable_result',
        },
      }),
      { label: 'Failed', variant: 'red' }
    )
  })

  test('keeps a successful consume outcome as Consume', () => {
    assert.deepEqual(
      getLogStatusDisplay(consumeType, {
        request_outcome: { status: 'success' },
      }),
      { label: 'Consume', variant: 'green' }
    )
  })

  test('keeps historical logs without an outcome on their type status', () => {
    assert.deepEqual(getLogStatusDisplay(consumeType, null), {
      label: 'Consume',
      variant: 'green',
    })
  })
})

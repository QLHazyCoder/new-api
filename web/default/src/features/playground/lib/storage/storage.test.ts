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
import { test } from 'node:test'

import {
  loadImageDeleteConfirmationDisabled,
  removeDeprecatedImageTaskStorage,
  saveImageDeleteConfirmationDisabled,
} from './storage'

test('removes only deprecated browser image task history', () => {
  const originalLocalStorage = Object.getOwnPropertyDescriptor(
    globalThis,
    'localStorage'
  )
  const values = new Map([
    ['playground_image_tasks', '[{"id":"legacy-task"}]'],
    ['playground_image_config', '{"n":2}'],
  ])

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem(key: string) {
        return values.get(key) ?? null
      },
      removeItem(key: string) {
        values.delete(key)
      },
      setItem(key: string, value: string) {
        values.set(key, value)
      },
    },
  })

  try {
    removeDeprecatedImageTaskStorage()

    assert.equal(values.has('playground_image_tasks'), false)
    assert.equal(values.get('playground_image_config'), '{"n":2}')
  } finally {
    if (originalLocalStorage) {
      Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
    } else {
      Reflect.deleteProperty(globalThis, 'localStorage')
    }
  }
})

test('persists the image delete confirmation preference separately from task history', () => {
  const originalLocalStorage = Object.getOwnPropertyDescriptor(
    globalThis,
    'localStorage'
  )
  const values = new Map<string, string>()

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem(key: string) {
        return values.get(key) ?? null
      },
      removeItem(key: string) {
        values.delete(key)
      },
      setItem(key: string, value: string) {
        values.set(key, value)
      },
    },
  })

  try {
    assert.equal(loadImageDeleteConfirmationDisabled(), false)

    saveImageDeleteConfirmationDisabled(true)
    assert.equal(loadImageDeleteConfirmationDisabled(), true)

    saveImageDeleteConfirmationDisabled(false)
    assert.equal(loadImageDeleteConfirmationDisabled(), false)
  } finally {
    if (originalLocalStorage) {
      Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
    } else {
      Reflect.deleteProperty(globalThis, 'localStorage')
    }
  }
})

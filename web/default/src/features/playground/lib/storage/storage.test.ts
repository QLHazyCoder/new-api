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

import { DEFAULT_IMAGE_CONFIG } from '../../constants'
import type { ImageTask } from '../../types'
import { normalizeLegacyImageTasks, removePlaygroundImageTask } from './storage'

const now = Date.UTC(2026, 6, 26)

function createTask(overrides: Partial<ImageTask>): ImageTask {
  return {
    id: 'legacy-task',
    prompt: 'draw a test image',
    config: DEFAULT_IMAGE_CONFIG,
    status: 'done',
    createdAt: now - 1000,
    ...overrides,
  }
}

describe('playground legacy image task migration', () => {
  test('keeps only local tasks created within seven days', () => {
    const tasks = normalizeLegacyImageTasks(
      [
        createTask({ id: 'recent' }),
        createTask({
          id: 'expired',
          createdAt: now - 8 * 24 * 60 * 60 * 1000,
        }),
        createTask({ id: 'server', origin: 'server' }),
      ],
      now
    )

    assert.deepEqual(
      tasks.map((task) => task.id),
      ['recent']
    )
    assert.equal(tasks[0].origin, 'legacy')
  })

  test('marks old active browser tasks as interrupted', () => {
    const [task] = normalizeLegacyImageTasks(
      [createTask({ status: 'running' })],
      now
    )

    assert.equal(task.status, 'interrupted')
    assert.equal(task.finishedAt, now)
  })

  test('allows browser-only history to be removed locally', () => {
    const [task] = normalizeLegacyImageTasks(
      [createTask({ id: 'deletable' })],
      now
    )

    assert.deepEqual(removePlaygroundImageTask([task], task.id), [])
  })
})

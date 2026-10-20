/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import type { TaskLog } from '../../types'
import { TaskDetailsDialog } from '../dialogs/task-details-dialog'

function taskLog(overrides: Partial<TaskLog> = {}): TaskLog {
  return {
    id: 1,
    user_id: 7,
    platform: 'zhiqi-video',
    task_id: 'task-public',
    action: 'MUSIC',
    channel_id: 357,
    group: 'default',
    quota: 25000,
    submit_time: 1,
    status: 'SUCCESS',
    ...overrides,
  }
}

function renderDetails(log: TaskLog, isAdmin = false) {
  return render(
    <TaskDetailsDialog
      log={log}
      isAdmin={isAdmin}
      isRoot={false}
      open
      onOpenChange={vi.fn()}
    />
  )
}

test('omits the platform row when the API hides a plugin platform', () => {
  renderDetails(taskLog({ platform: undefined }))

  expect(screen.queryByText('Platform')).not.toBeInTheDocument()
  expect(screen.queryByText('zhiqi-video')).not.toBeInTheDocument()
})

test('keeps the platform row when a platform is present', () => {
  renderDetails(taskLog({ platform: 'suno' }), true)

  expect(screen.getByText('Platform')).toBeInTheDocument()
  expect(screen.getByText('suno')).toBeInTheDocument()
})

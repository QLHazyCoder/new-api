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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { TaskLog } from '../../types'
import { useTaskLogsColumns } from '../columns/task-logs-columns'

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

function TaskIdCell({ log }: { log: TaskLog }) {
  const table = useReactTable({
    data: [log],
    columns: useTaskLogsColumns(false, false),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'task_id')

  if (!cell) throw new Error('The task ID column must exist')
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}

test('shows only the task action when a plugin platform is omitted', () => {
  render(<TaskIdCell log={taskLog({ platform: undefined })} />)

  expect(screen.getByText('Generate Music')).toBeInTheDocument()
  expect(screen.queryByText('zhiqi-video')).not.toBeInTheDocument()
  expect(screen.queryByText(/·/)).not.toBeInTheDocument()
})

test('keeps native platform labels in the task ID cell', () => {
  render(<TaskIdCell log={taskLog({ platform: 'suno' })} />)

  expect(screen.getByText('suno · Generate Music')).toBeInTheDocument()
})

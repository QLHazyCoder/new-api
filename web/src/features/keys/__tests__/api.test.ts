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
import { afterEach, describe, expect, test, vi } from 'vitest'

const { apiGetMock } = vi.hoisted(() => ({ apiGetMock: vi.fn() }))

vi.mock('@/lib/api', () => ({ api: { get: apiGetMock } }))

const { getApiKeyModels } = await import('../api')

afterEach(() => {
  vi.clearAllMocks()
})

describe('getApiKeyModels', () => {
  test('uses the selected key to read the authoritative relay model list', async () => {
    apiGetMock.mockResolvedValue({
      data: {
        success: true,
        data: [
          { id: 'group-model' },
          { id: ' group-model ' },
          { id: 'second-model' },
          { id: 42 },
        ],
      },
    })

    await expect(getApiKeyModels('key-value')).resolves.toEqual([
      'group-model',
      'second-model',
    ])
    expect(apiGetMock).toHaveBeenCalledWith(
      '/v1/models',
      expect.objectContaining({
        headers: { Authorization: 'Bearer sk-key-value' },
        disableDuplicate: true,
        skipAuthRefresh: true,
        skipBusinessError: true,
        skipErrorHandler: true,
        skipSessionAuthorization: true,
      })
    )
  })

  test('returns no options when the relay model list is unsuccessful', async () => {
    apiGetMock.mockResolvedValue({ data: { success: false } })

    await expect(getApiKeyModels('sk-key-value')).resolves.toEqual([])
  })
})

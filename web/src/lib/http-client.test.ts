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
import { afterEach, describe, expect, test } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { api } from './http-client'

function setSessionAccessToken(accessToken: string | null) {
  useAuthStore.setState((state) => ({
    ...state,
    auth: { ...state.auth, accessToken },
  }))
}

afterEach(() => {
  setSessionAccessToken(null)
})

describe('HTTP client authorization', () => {
  test('preserves explicit token authorization when session injection is disabled', async () => {
    setSessionAccessToken('session-token')

    const response = await api.get('/http-client-token-auth-test', {
      disableDuplicate: true,
      headers: { Authorization: 'Bearer api-key-token' },
      skipSessionAuthorization: true,
      adapter: async (config) => ({
        config,
        data: { success: true },
        headers: {},
        status: 200,
        statusText: 'OK',
      }),
    })

    expect(response.config.headers.get('Authorization')).toBe(
      'Bearer api-key-token'
    )
  })

  test('uses the session authorization by default', async () => {
    setSessionAccessToken('session-token')

    const response = await api.get('/http-client-session-auth-test', {
      disableDuplicate: true,
      adapter: async (config) => ({
        config,
        data: { success: true },
        headers: {},
        status: 200,
        statusText: 'OK',
      }),
    })

    expect(response.config.headers.get('Authorization')).toBe(
      'Bearer session-token'
    )
  })
})

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

import { api } from '@/lib/api'

import { getPublicDocArticle, getPublicDocsTree } from '../api'
import type { PublicDocNode } from '../types'

type Response = { success: true; data: unknown }
type ApiOptions = {
  skipAuthRefresh?: boolean
  skipErrorHandler?: boolean
  skipSessionAuthorization?: boolean
}
type MockableApi = {
  get: (url: string, options?: ApiOptions) => Promise<{ data: Response }>
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const requests: string[] = []
const options: ApiOptions[] = []

afterEach(() => {
  requests.length = 0
  options.length = 0
  apiClient.get = originalGet
})

describe('public documentation API paths', () => {
  test('the directory request uses the public same-origin route', async () => {
    const tree: PublicDocNode[] = []
    apiClient.get = async (url, requestOptions) => {
      requests.push(url)
      options.push(requestOptions ?? {})
      return { data: { success: true, data: tree } }
    }

    await expect(getPublicDocsTree()).resolves.toBe(tree)
    expect(requests).toEqual(['/api/public/docs'])
    expect(options).toEqual([
      {
        skipSessionAuthorization: true,
        skipAuthRefresh: true,
        skipErrorHandler: true,
      },
    ])
  })

  test('article requests keep the slug on the public same-origin route', async () => {
    const article = { slug: 'quick-start' }
    apiClient.get = async (url, requestOptions) => {
      requests.push(url)
      options.push(requestOptions ?? {})
      return { data: { success: true, data: article } }
    }

    await expect(getPublicDocArticle('quick-start')).resolves.toBe(article)
    expect(requests).toEqual(['/api/public/docs/quick-start'])
    expect(options[0]?.skipSessionAuthorization).toBe(true)
    expect(options[0]?.skipAuthRefresh).toBe(true)
  })
})

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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  PublicDocArticle,
  PublicDocNode,
  PublicDocsResponse,
} from './types'

export const publicDocsTreeQueryKey = ['public-docs', 'tree'] as const

export async function getPublicDocsTree(): Promise<PublicDocNode[]> {
  const response = await api.get<PublicDocsResponse<PublicDocNode[] | null>>(
    '/api/public/docs',
    {
      skipSessionAuthorization: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
    }
  )
  return requireServerSuccess(response.data).data ?? []
}

export async function getPublicDocArticle(
  slug: string
): Promise<PublicDocArticle> {
  const response = await api.get<PublicDocsResponse<PublicDocArticle>>(
    `/api/public/docs/${encodeURIComponent(slug)}`,
    {
      skipSessionAuthorization: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
    }
  )
  return requireServerSuccess(response.data).data
}

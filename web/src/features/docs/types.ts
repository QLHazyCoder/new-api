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
export type PublicDocArticleNode = {
  type: 'article'
  id: number
  slug: string
  title: string
  sort_order: number
}

export type PublicDocFolderNode = {
  type: 'folder'
  id: number
  name: string
  sort_order: number
  children: PublicDocNode[]
}

export type PublicDocNode = PublicDocArticleNode | PublicDocFolderNode

export type PublicDocArticle = {
  slug: string
  title: string
  markdown: string
  updated_at: string
  path: string[]
}

export type PublicDocsResponse<T> = {
  success: boolean
  data: T
  message?: string
  code?: string
}

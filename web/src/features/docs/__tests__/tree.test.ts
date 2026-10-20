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
import { describe, expect, test } from 'vitest'

import { filterPublicDocsTree, firstPublishedDocSlug } from '../tree'
import type { PublicDocNode } from '../types'

const tree: PublicDocNode[] = [
  {
    type: 'folder',
    id: 1,
    name: 'Guides',
    sort_order: 0,
    children: [
      {
        type: 'article',
        id: 2,
        slug: 'quick-start',
        title: 'Quick Start',
        sort_order: 0,
      },
      {
        type: 'folder',
        id: 3,
        name: 'API',
        sort_order: 10,
        children: [
          {
            type: 'article',
            id: 4,
            slug: 'api-reference',
            title: 'API Reference',
            sort_order: 0,
          },
        ],
      },
    ],
  },
]

describe('public documentation tree', () => {
  test('root navigation selects the first article in directory order', () => {
    expect(firstPublishedDocSlug(tree)).toBe('quick-start')
  })

  test('search keeps matching articles and their ancestor folders', () => {
    const filtered = filterPublicDocsTree(tree, 'reference')
    expect(filtered).toHaveLength(1)
    expect(filtered[0].type).toBe('folder')
    if (filtered[0].type !== 'folder') return
    expect(filtered[0].children).toHaveLength(1)
    expect(filtered[0].children[0].type).toBe('folder')
    if (filtered[0].children[0].type !== 'folder') return
    expect(filtered[0].children[0].children).toEqual([
      {
        type: 'article',
        id: 4,
        slug: 'api-reference',
        title: 'API Reference',
        sort_order: 0,
      },
    ])
  })

  test('empty search preserves the complete published tree', () => {
    expect(filterPublicDocsTree(tree, '')).toEqual(tree)
  })
})

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
import type { PublicDocNode } from './types'

export function firstPublishedDocSlug(nodes: PublicDocNode[]): string | null {
  for (const node of nodes) {
    if (node.type === 'article') return node.slug
    const nestedSlug = firstPublishedDocSlug(node.children)
    if (nestedSlug) return nestedSlug
  }
  return null
}

export function filterPublicDocsTree(
  nodes: PublicDocNode[],
  query: string
): PublicDocNode[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return nodes

  const filtered: PublicDocNode[] = []
  for (const node of nodes) {
    if (node.type === 'article') {
      if (`${node.title} ${node.slug}`.toLowerCase().includes(needle)) {
        filtered.push(node)
      }
      continue
    }

    if (node.name.toLowerCase().includes(needle)) {
      filtered.push(node)
      continue
    }
    const children = filterPublicDocsTree(node.children, needle)
    if (children.length) filtered.push({ ...node, children })
  }
  return filtered
}

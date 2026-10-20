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
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  File01Icon,
  Folder01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import type { PublicDocNode } from './types'

interface DocumentTreeProps {
  nodes: PublicDocNode[]
  expandedFolders: ReadonlySet<number>
  searchActive: boolean
  selectedSlug: string | null
  onToggleFolder: (id: number) => void
  onSelectArticle: (slug: string) => void
}

export function DocumentTree(props: DocumentTreeProps) {
  return (
    <ul className='flex flex-col gap-0.5'>
      {props.nodes.map((node) => {
        if (node.type === 'article') {
          const active = props.selectedSlug === node.slug
          return (
            <li key={`article-${node.id}`}>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                aria-current={active ? 'page' : undefined}
                onClick={() => props.onSelectArticle(node.slug)}
                className={cn(
                  'h-9 w-full justify-start gap-2 px-2 text-left font-normal',
                  active && 'bg-accent text-accent-foreground font-medium'
                )}
              >
                <HugeiconsIcon
                  icon={File01Icon}
                  data-icon='inline-start'
                  strokeWidth={2}
                  aria-hidden='true'
                />
                <span className='min-w-0 flex-1 truncate'>{node.title}</span>
              </Button>
            </li>
          )
        }

        const isExpanded =
          props.searchActive || props.expandedFolders.has(node.id)
        return (
          <li key={`folder-${node.id}`}>
            <div className='flex min-w-0 items-center gap-1'>
              {props.searchActive ? (
                <span className='text-muted-foreground flex size-7 shrink-0 items-center justify-center'>
                  <HugeiconsIcon
                    icon={ArrowDown01Icon}
                    className='size-4'
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </span>
              ) : (
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={node.name}
                  aria-expanded={isExpanded}
                  title={node.name}
                  onClick={() => props.onToggleFolder(node.id)}
                  className='size-7 shrink-0'
                >
                  <HugeiconsIcon
                    icon={isExpanded ? ArrowDown01Icon : ArrowRight01Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </Button>
              )}
              <span className='flex h-9 min-w-0 flex-1 items-center gap-2 px-1 text-sm font-medium'>
                <HugeiconsIcon
                  icon={Folder01Icon}
                  strokeWidth={2}
                  className='text-primary size-4 shrink-0'
                  aria-hidden='true'
                />
                <span className='truncate'>{node.name}</span>
              </span>
            </div>
            {isExpanded && node.children.length > 0 && (
              <div className='border-border ms-3 border-s ps-2'>
                <DocumentTree
                  nodes={node.children}
                  expandedFolders={props.expandedFolders}
                  searchActive={props.searchActive}
                  selectedSlug={props.selectedSlug}
                  onToggleFolder={props.onToggleFolder}
                  onSelectArticle={props.onSelectArticle}
                />
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}

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
import { Search01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'

import { DocumentTree } from './document-tree'
import { filterPublicDocsTree } from './tree'
import type { PublicDocNode } from './types'

interface DirectoryPanelProps {
  id: string
  tree: PublicDocNode[]
  search: string
  onSearchChange: (value: string) => void
  expandedFolders: ReadonlySet<number>
  selectedSlug: string | null
  loading: boolean
  error: boolean
  onRetry: () => void
  onToggleFolder: (id: number) => void
  onSelectArticle: (slug: string) => void
}

export function DirectoryPanel(props: DirectoryPanelProps) {
  const { t } = useTranslation()
  const filteredTree = useMemo(
    () => filterPublicDocsTree(props.tree, props.search),
    [props.search, props.tree]
  )
  let content: ReactNode
  if (props.loading) {
    content = <LoadingState size='sm' className='min-h-24' />
  } else if (props.error) {
    content = (
      <ErrorState
        title={t('Unable to load documentation')}
        onRetry={props.onRetry}
        className='min-h-48 px-2'
      />
    )
  } else if (filteredTree.length) {
    content = (
      <nav aria-label={t('Documentation directory')}>
        <DocumentTree
          nodes={filteredTree}
          expandedFolders={props.expandedFolders}
          searchActive={Boolean(props.search.trim())}
          selectedSlug={props.selectedSlug}
          onToggleFolder={props.onToggleFolder}
          onSelectArticle={props.onSelectArticle}
        />
      </nav>
    )
  } else {
    content = (
      <p className='text-muted-foreground px-2 py-4 text-sm'>
        {props.tree.length
          ? t('No matching documents found')
          : t('No published documentation yet')}
      </p>
    )
  }

  return (
    <>
      <div className='border-border border-b p-3'>
        <label htmlFor={props.id} className='sr-only'>
          {t('Search documentation')}
        </label>
        <div className='relative'>
          <HugeiconsIcon
            icon={Search01Icon}
            strokeWidth={2}
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            id={props.id}
            type='search'
            value={props.search}
            onChange={(event) => props.onSearchChange(event.target.value)}
            placeholder={t('Search documentation')}
            className='h-9 ps-9'
          />
        </div>
      </div>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='p-3'>{content}</div>
      </ScrollArea>
    </>
  )
}

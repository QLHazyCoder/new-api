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
import { Cancel01Icon, Menu01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

import { getPublicDocsTree, publicDocsTreeQueryKey } from './api'
import { DirectoryPanel } from './directory-panel'
import type { PublicDocNode } from './types'

function folderIDs(nodes: PublicDocNode[]): number[] {
  return nodes.flatMap((node) =>
    node.type === 'folder' ? [node.id, ...folderIDs(node.children)] : []
  )
}

export function DocsLayout(props: { children: ReactNode }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const selectedSlug = pathname.startsWith('/docs/')
    ? pathname.slice('/docs/'.length).split('/')[0]
    : null
  const treeQuery = useQuery({
    queryKey: publicDocsTreeQueryKey,
    queryFn: getPublicDocsTree,
    staleTime: 15_000,
    meta: { errorToast: false },
  })
  const [search, setSearch] = useState('')
  const [expandedFolders, setExpandedFolders] = useState<Set<number>>(new Set())
  const [mobileDirectoryOpen, setMobileDirectoryOpen] = useState(false)
  const initializedExpanded = useRef(false)
  const tree = treeQuery.data ?? []

  useEffect(() => {
    if (!treeQuery.data || initializedExpanded.current) return
    initializedExpanded.current = true
    setExpandedFolders(new Set(folderIDs(treeQuery.data)))
  }, [treeQuery.data])

  const toggleFolder = (id: number) => {
    setExpandedFolders((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const selectArticle = (slug: string) => {
    setMobileDirectoryOpen(false)
    void navigate({ to: '/docs/$slug', params: { slug } })
  }

  const panelProps = {
    tree,
    search,
    onSearchChange: setSearch,
    expandedFolders,
    selectedSlug,
    loading: treeQuery.isLoading,
    error: treeQuery.isError,
    onRetry: () => void treeQuery.refetch(),
    onToggleFolder: toggleFolder,
    onSelectArticle: selectArticle,
  }

  return (
    <PublicLayout showMainContainer={false}>
      <Sheet open={mobileDirectoryOpen} onOpenChange={setMobileDirectoryOpen}>
        <div className='min-h-svh pt-16'>
          <div className='mx-auto grid min-h-[calc(100svh-4rem)] w-full max-w-[1800px] lg:grid-cols-[280px_minmax(0,1fr)]'>
            <aside className='bg-muted/20 border-border sticky top-16 hidden h-[calc(100svh-4rem)] min-h-0 flex-col border-e lg:flex'>
              <DirectoryPanel id='docs-search-desktop' {...panelProps} />
            </aside>

            <main className='min-w-0'>
              <div className='border-border flex h-12 items-center border-b px-3 lg:hidden'>
                <SheetTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      size='icon-sm'
                      aria-label={t('Open documentation directory')}
                      title={t('Open documentation directory')}
                    />
                  }
                >
                  <HugeiconsIcon
                    icon={Menu01Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </SheetTrigger>
              </div>
              {props.children}
            </main>
          </div>
        </div>

        <SheetContent side='left' showCloseButton={false} className='gap-0 p-0'>
          <SheetHeader className='border-border flex-row items-center justify-between border-b'>
            <SheetTitle>{t('Documentation directory')}</SheetTitle>
            <SheetClose
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Close documentation directory')}
                  title={t('Close documentation directory')}
                />
              }
            >
              <HugeiconsIcon
                icon={Cancel01Icon}
                strokeWidth={2}
                aria-hidden='true'
              />
            </SheetClose>
          </SheetHeader>
          <DirectoryPanel id='docs-search-mobile' {...panelProps} />
        </SheetContent>
      </Sheet>
    </PublicLayout>
  )
}

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
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

import type { DocumentationHeading } from './markdown'

interface ArticleTocListProps {
  activeHeadingId: string | null
  headings: readonly DocumentationHeading[]
  onSelect: (id: string) => void
}

function ArticleTocList(props: ArticleTocListProps) {
  return (
    <ol className='flex flex-col gap-1 border-s ps-3'>
      {props.headings.map((heading) => {
        const active = props.activeHeadingId === heading.id
        return (
          <li key={heading.id}>
            <a
              href={`#${encodeURIComponent(heading.id)}`}
              aria-current={active ? 'location' : undefined}
              onClick={(event) => {
                if (
                  event.metaKey ||
                  event.ctrlKey ||
                  event.shiftKey ||
                  event.altKey
                ) {
                  return
                }
                event.preventDefault()
                props.onSelect(heading.id)
              }}
              className={cn(
                'text-muted-foreground hover:text-foreground block rounded-sm py-1 text-sm leading-5 wrap-anywhere transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                heading.depth === 3 && 'ps-3 text-xs',
                active && 'text-primary font-medium'
              )}
            >
              {heading.text}
            </a>
          </li>
        )
      })}
    </ol>
  )
}

export function ArticleToc(props: ArticleTocListProps) {
  const { t } = useTranslation()

  if (!props.headings.length) return null

  return (
    <aside className='sticky top-24 hidden max-h-[calc(100svh-7rem)] w-[240px] shrink-0 overflow-y-auto px-6 pt-12 pb-8 xl:block'>
      <nav aria-label={t('Article navigation')}>
        <p className='text-foreground mb-3 text-xs font-semibold'>
          {t('On this page')}
        </p>
        <ArticleTocList {...props} />
      </nav>
    </aside>
  )
}

export function MobileArticleToc(props: ArticleTocListProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const selectedHeading = useRef<string | null>(null)

  if (!props.headings.length) return null

  const selectHeading = (id: string) => {
    selectedHeading.current = id
    setOpen(false)
  }

  return (
    <div className='pb-4 xl:hidden'>
      <Sheet
        open={open}
        onOpenChange={(nextOpen) => {
          if (nextOpen) selectedHeading.current = null
          setOpen(nextOpen)
        }}
        onOpenChangeComplete={(isOpen) => {
          if (!isOpen && selectedHeading.current) {
            // Wait for the scroll lock and focus trap to close before jumping.
            props.onSelect(selectedHeading.current)
          }
        }}
      >
        <SheetTrigger
          render={
            <Button
              type='button'
              variant='outline'
              size='sm'
              aria-label={t('Open article navigation')}
            />
          }
        >
          <HugeiconsIcon
            icon={Menu01Icon}
            data-icon='inline-start'
            strokeWidth={2}
            aria-hidden='true'
          />
          {t('On this page')}
        </SheetTrigger>
        <SheetContent
          side='right'
          showCloseButton={false}
          finalFocus={() =>
            selectedHeading.current
              ? document.getElementById(selectedHeading.current)
              : true
          }
          className='gap-0 p-0'
        >
          <SheetHeader className='border-border flex-row items-center justify-between border-b'>
            <SheetTitle>{t('Article navigation')}</SheetTitle>
            <SheetClose
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Close article navigation')}
                  title={t('Close article navigation')}
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
          <div className='min-h-0 flex-1 overflow-y-auto p-4'>
            <ArticleTocList
              activeHeadingId={props.activeHeadingId}
              headings={props.headings}
              onSelect={selectHeading}
            />
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}

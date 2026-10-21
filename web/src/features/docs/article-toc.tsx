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
import { useTranslation } from 'react-i18next'

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
                heading.depth === 3 && 'ps-3',
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
    <aside className='border-border sticky top-24 hidden max-h-[calc(100svh-7rem)] w-[380px] shrink-0 overflow-y-auto border-s px-6 py-8 xl:block'>
      <nav aria-label={t('Article navigation')}>
        <p className='text-foreground mb-3 text-sm font-semibold'>
          {t('On this page')}
        </p>
        <ArticleTocList {...props} />
      </nav>
    </aside>
  )
}

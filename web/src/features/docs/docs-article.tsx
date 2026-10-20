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
import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { toIntlLocale } from '@/i18n/languages'
import { getServerErrorStatus } from '@/lib/server-error-message'

import { getPublicDocArticle } from './api'
import { DocsMarkdown } from './markdown-view'

export function DocsArticlePage(props: { slug: string }) {
  const { i18n, t } = useTranslation()
  const query = useQuery({
    queryKey: ['public-docs', 'article', props.slug],
    queryFn: () => getPublicDocArticle(props.slug),
    staleTime: 15_000,
    meta: { errorToast: false },
  })
  const article = query.data

  useEffect(() => {
    document.title = article ? `${article.title} - ${t('Docs')}` : t('Docs')
  }, [article, t])

  if (query.isLoading) {
    return <LoadingState className='min-h-[60svh]' />
  }

  if (query.isError) {
    if (getServerErrorStatus(query.error) === 404) {
      return (
        <div className='px-5 py-12 sm:px-8'>
          <EmptyState
            title={t('Document not found')}
            description={t(
              'This document is unavailable or has not been published.'
            )}
            className='min-h-[50svh]'
          />
        </div>
      )
    }

    return (
      <ErrorState
        title={t('Unable to load documentation')}
        onRetry={() => void query.refetch()}
        className='min-h-[60svh]'
      />
    )
  }

  if (!article) return null

  const updatedAt = new Intl.DateTimeFormat(
    toIntlLocale(i18n.resolvedLanguage || i18n.language),
    { dateStyle: 'medium', timeStyle: 'short' }
  ).format(new Date(article.updated_at))

  return (
    <article className='mx-auto w-full max-w-[800px] px-5 py-8 sm:px-8 sm:py-12'>
      <nav
        aria-label={t('Document location')}
        className='text-muted-foreground mb-5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs'
      >
        {article.path.map((part, index) => (
          <span
            key={article.path.slice(0, index + 1).join('/')}
            className='flex items-center gap-2'
          >
            <span>{part}</span>
            {index < article.path.length - 1 && (
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                strokeWidth={2}
                className='size-3'
                aria-hidden='true'
              />
            )}
          </span>
        ))}
      </nav>
      <h1 className='text-foreground text-2xl leading-tight font-semibold break-words sm:text-[30px]'>
        {article.title}
      </h1>
      <p className='text-muted-foreground border-border mt-3 border-b pb-5 text-xs'>
        <time dateTime={article.updated_at}>
          {t('Updated {{date}}', { date: updatedAt })}
        </time>
      </p>
      <div className='pt-3'>
        <DocsMarkdown markdown={article.markdown} />
      </div>
    </article>
  )
}

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
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'

import { getPublicDocsTree, publicDocsTreeQueryKey } from './api'
import { firstPublishedDocSlug } from './tree'

export function DocsIndexPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const query = useQuery({
    queryKey: publicDocsTreeQueryKey,
    queryFn: getPublicDocsTree,
    staleTime: 15_000,
    meta: { errorToast: false },
  })
  const firstSlug = query.data ? firstPublishedDocSlug(query.data) : null

  useEffect(() => {
    if (firstSlug) {
      void navigate({
        to: '/docs/$slug',
        params: { slug: firstSlug },
        replace: true,
      })
    }
  }, [firstSlug, navigate])

  if (query.isLoading || firstSlug) {
    return <LoadingState className='min-h-[60svh]' />
  }

  if (query.isError) {
    return (
      <ErrorState
        title={t('Unable to load documentation')}
        onRetry={() => void query.refetch()}
        className='min-h-[60svh]'
      />
    )
  }

  return (
    <div className='flex min-h-[60svh] items-center justify-center px-5 py-12'>
      <EmptyState
        title={t('No published documentation yet')}
        className='min-h-0'
      />
    </div>
  )
}

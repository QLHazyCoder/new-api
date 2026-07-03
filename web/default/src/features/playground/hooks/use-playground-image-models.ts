import { useQuery } from '@tanstack/react-query'
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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { getUserModels } from '../api'
import { getOptionLoadErrorMessage } from '../lib'

export function usePlaygroundImageModels(currentGroup: string) {
  const { t } = useTranslation()

  const {
    data: imageModelsData,
    error: imageModelsError,
    isError: isImageModelsError,
    isLoading: isLoadingImageModels,
  } = useQuery({
    queryKey: ['playground-image-models', currentGroup],
    queryFn: () => getUserModels(currentGroup),
    enabled: currentGroup !== '',
  })

  useEffect(() => {
    if (!isImageModelsError) return

    toast.error(
      getOptionLoadErrorMessage(
        imageModelsError,
        t('Failed to load playground models')
      )
    )
  }, [isImageModelsError, imageModelsError, t])

  return {
    imageModelOptions: imageModelsData ?? [],
    isLoadingImageModels,
  }
}

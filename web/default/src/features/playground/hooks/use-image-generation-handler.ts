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
import { nanoid } from 'nanoid'
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  createImageEditBatch,
  createImageGenerationBatch,
  deleteImageTask as deleteServerImageTask,
  getAllImageTasks,
  retryImageTask as retryServerImageTask,
} from '../api'
import { ERROR_MESSAGES } from '../constants'
import {
  buildImageEditFormData,
  buildImageGenerationPayload,
  findImageModelCapabilities,
  normalizePlaygroundImageConfig,
  normalizeImageGenerationCount,
  removePlaygroundImageTask,
} from '../lib'
import type {
  ImageGenerationConfig,
  ImageGroupOption,
  ImageReferenceInput,
  ImageTask,
  ImageTaskStatus,
  ServerImageTask,
} from '../types'

interface UseImageGenerationHandlerOptions {
  config: ImageGenerationConfig
  enabled: boolean
  groups: ImageGroupOption[]
  tasks: ImageTask[]
  onTasksUpdate: (
    updater: ImageTask[] | ((prev: ImageTask[]) => ImageTask[])
  ) => void
}

function getImageGenerationError(
  error: unknown,
  forbiddenMessage: string
): {
  message: string
  code?: string
} {
  const err = error as {
    response?: {
      data?: {
        message?: string
        error?: {
          code?: string
          message?: string
        }
      }
    }
    message?: string
  }

  const upstreamMessage =
    err?.response?.data?.error?.message ||
    err?.response?.data?.message ||
    err?.message ||
    ''
  const normalizedMessage = upstreamMessage.toLowerCase()
  const isForbiddenUpstream =
    normalizedMessage.includes('forbidden') ||
    normalizedMessage.includes('access denied') ||
    normalizedMessage.includes('access forbidden')

  if (isForbiddenUpstream) {
    return {
      message: forbiddenMessage,
      code: err?.response?.data?.error?.code || undefined,
    }
  }

  return {
    message: upstreamMessage || ERROR_MESSAGES.API_REQUEST_ERROR,
    code: err?.response?.data?.error?.code || undefined,
  }
}

function mapServerImageTaskStatus(
  status: ServerImageTask['status']
): ImageTaskStatus {
  if (status === 'succeeded') return 'done'
  if (status === 'failed') return 'error'
  return status
}

function mapServerImageTask(task: ServerImageTask): ImageTask {
  const config = {
    size: '',
    aspect_ratio: '',
    resolution: '',
    quality: 'auto',
    ...task.config,
    model: task.model,
    group: task.group,
    n: 1,
    response_format: 'url',
  } as ImageGenerationConfig

  return {
    id: task.id,
    batchId: task.batch_id,
    taskIndex: task.task_index,
    prompt: task.prompt,
    config,
    mode: task.mode,
    status: mapServerImageTaskStatus(task.status),
    image: task.image
      ? {
          url: task.image.url,
          mime_type: task.image.mime_type,
        }
      : undefined,
    downloadUrl: task.image?.download_url,
    error: task.error,
    errorCode: task.error_code,
    origin: 'server',
    createdAt: task.created_at * 1000,
    startedAt: task.started_at ? task.started_at * 1000 : undefined,
    finishedAt: task.finished_at ? task.finished_at * 1000 : undefined,
    expiresAt: task.expires_at * 1000,
  }
}

function isActiveImageTask(task: ImageTask): boolean {
  return ['queued', 'running', 'saving'].includes(task.status)
}

export function useImageGenerationHandler({
  config,
  enabled,
  groups,
  tasks,
  onTasksUpdate,
}: UseImageGenerationHandlerOptions) {
  const { t } = useTranslation()
  const refreshRequestRef = useRef<Promise<void> | null>(null)
  const hasActiveTasksRef = useRef(tasks.some(isActiveImageTask))
  const hasLoadedTasksRef = useRef(false)

  useEffect(() => {
    hasActiveTasksRef.current = tasks.some(isActiveImageTask)
  }, [tasks])

  const refreshTasks = useCallback(
    async (force = false) => {
      if (refreshRequestRef.current) {
        if (!force) return refreshRequestRef.current
        await refreshRequestRef.current
      }

      const request = (async () => {
        const serverTasks = (await getAllImageTasks()).map(mapServerImageTask)
        hasLoadedTasksRef.current = true
        hasActiveTasksRef.current = serverTasks.some(isActiveImageTask)
        onTasksUpdate((previous) => {
          const legacyTasks = previous.filter(
            (task) => task.origin !== 'server'
          )
          return [...serverTasks, ...legacyTasks].sort(
            (left, right) => right.createdAt - left.createdAt
          )
        })
      })()
      refreshRequestRef.current = request
      try {
        await request
      } finally {
        if (refreshRequestRef.current === request) {
          refreshRequestRef.current = null
        }
      }
    },
    [onTasksUpdate]
  )

  useEffect(() => {
    if (!enabled) return
    const refreshSilently = (force = false) => {
      void refreshTasks(force).catch(() => undefined)
    }
    hasLoadedTasksRef.current = false
    refreshSilently()

    const refreshWhenVisible = () => {
      if (document.visibilityState === 'visible') {
        refreshSilently(true)
      }
    }
    const interval = window.setInterval(() => {
      if (
        document.visibilityState === 'visible' &&
        (hasActiveTasksRef.current || !hasLoadedTasksRef.current)
      ) {
        refreshSilently()
      }
    }, 2000)
    document.addEventListener('visibilitychange', refreshWhenVisible)
    window.addEventListener('focus', refreshWhenVisible)
    return () => {
      window.clearInterval(interval)
      document.removeEventListener('visibilitychange', refreshWhenVisible)
      window.removeEventListener('focus', refreshWhenVisible)
    }
  }, [enabled, refreshTasks])

  const generateImage = useCallback(
    async (
      prompt: string,
      referenceImages: ImageReferenceInput[] = [],
      overrideConfig?: ImageGenerationConfig
    ) => {
      const trimmedPrompt = prompt.trim()
      const requestedConfig = overrideConfig ?? config
      const capabilities = findImageModelCapabilities(groups, requestedConfig)
      if (!capabilities) {
        toast.error(t('Please select an image model'))
        return
      }
      const sourceConfig = normalizePlaygroundImageConfig(
        requestedConfig,
        capabilities
      )
      const requestedCount = normalizeImageGenerationCount(sourceConfig.n)
      const effectiveConfig = {
        ...sourceConfig,
        n: requestedCount,
      }

      if (!trimmedPrompt) {
        toast.error(t('Please enter an image prompt'))
        return
      }

      if (!effectiveConfig.model) {
        toast.error(t('Please select an image model'))
        return
      }

      const isEditMode = referenceImages.length > 0
      if (isEditMode && !capabilities.supports_editing) {
        toast.error(
          t('The selected image model does not support reference images')
        )
        return
      }
      const clientBatchId = nanoid()
      try {
        if (isEditMode) {
          const formData = buildImageEditFormData(
            trimmedPrompt,
            { ...effectiveConfig, n: 1 },
            referenceImages,
            capabilities
          )
          formData.append('client_batch_id', clientBatchId)
          formData.append('count', String(requestedCount))
          await createImageEditBatch(formData)
        } else {
          await createImageGenerationBatch({
            ...buildImageGenerationPayload(
              trimmedPrompt,
              { ...effectiveConfig, n: 1 },
              capabilities
            ),
            client_batch_id: clientBatchId,
            count: requestedCount,
          })
        }
      } catch (error: unknown) {
        const parsed = getImageGenerationError(
          error,
          isEditMode
            ? t(
                'The selected channel does not support image editing for this model'
              )
            : t(
                'The selected channel does not have access to this image model, or the upstream does not support image generation for it'
              )
        )
        toast.error(parsed.message)
        return
      }
      hasActiveTasksRef.current = true
      await refreshTasks(true).catch(() => undefined)
    },
    [config, groups, refreshTasks, t]
  )

  const retryTask = useCallback(
    async (task: ImageTask) => {
      if (task.origin !== 'server') return
      if (task.mode === 'edit') {
        toast.error(t('Upload the reference images again to retry this edit'))
        return
      }
      try {
        await retryServerImageTask(task.id)
      } catch (error: unknown) {
        toast.error(getImageGenerationError(error, t('Request failed')).message)
        return
      }
      hasActiveTasksRef.current = true
      await refreshTasks(true).catch(() => undefined)
    },
    [refreshTasks, t]
  )

  const deleteTask = useCallback(
    async (task: ImageTask) => {
      if (task.origin !== 'server') {
        onTasksUpdate((previous) =>
          removePlaygroundImageTask(previous, task.id)
        )
        return
      }
      try {
        await deleteServerImageTask(task.id)
        onTasksUpdate((previous) =>
          removePlaygroundImageTask(previous, task.id)
        )
      } catch (error: unknown) {
        toast.error(getImageGenerationError(error, t('Request failed')).message)
        return
      }
      await refreshTasks(true).catch(() => undefined)
    },
    [onTasksUpdate, refreshTasks, t]
  )

  return {
    deleteTask,
    generateImage,
    refreshTasks,
    retryTask,
  }
}

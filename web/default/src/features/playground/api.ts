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
import { api } from '@/lib/api'

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ImageGenerationRequest,
  ImageGenerationResponse,
  ImageGroupOption,
  ImageModelCapabilities,
  ImageModelOption,
  ModelOption,
  GroupOption,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function sendImageGeneration(
  payload: ImageGenerationRequest
): Promise<ImageGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_GENERATIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function sendImageEdit(
  payload: FormData
): Promise<ImageGenerationResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_EDITS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group?: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: {
      with_endpoint_types: true,
      ...(group ? { group } : {}),
    },
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data
    .map((model: string | ModelOption): ModelOption | null => {
      if (typeof model === 'string') {
        return {
          label: model,
          value: model,
        }
      }

      if (!model || typeof model.value !== 'string') return null

      const supportedEndpointTypes =
        model.supportedEndpointTypes || model.supported_endpoint_types || []

      return {
        label: model.label || model.value,
        value: model.value,
        supported_endpoint_types: supportedEndpointTypes,
        supportedEndpointTypes,
      }
    })
    .filter(Boolean) as ModelOption[]
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

function normalizeImageCapabilities(
  capabilities: Partial<ImageModelCapabilities> | undefined
): ImageModelCapabilities {
  return {
    provider: capabilities?.provider || 'other',
    size_mode: capabilities?.size_mode || 'none',
    sizes: Array.isArray(capabilities?.sizes) ? capabilities.sizes : [],
    aspect_ratios: Array.isArray(capabilities?.aspect_ratios)
      ? capabilities.aspect_ratios
      : [],
    resolutions: Array.isArray(capabilities?.resolutions)
      ? capabilities.resolutions
      : [],
    qualities: Array.isArray(capabilities?.qualities)
      ? capabilities.qualities
      : [],
    output_formats: Array.isArray(capabilities?.output_formats)
      ? capabilities.output_formats
      : [],
    default_size: capabilities?.default_size,
    default_aspect_ratio: capabilities?.default_aspect_ratio,
    default_resolution: capabilities?.default_resolution,
    default_quality: capabilities?.default_quality,
    default_output_format: capabilities?.default_output_format,
    supports_editing: Boolean(capabilities?.supports_editing),
    supports_moderation: Boolean(capabilities?.supports_moderation),
    supports_output_compression: Boolean(
      capabilities?.supports_output_compression
    ),
    max_images: Math.max(1, capabilities?.max_images || 1),
  }
}

export async function getUserImageModelGroups(): Promise<ImageGroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_IMAGE_MODELS)
  const { data } = res
  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data
    .map((group: Partial<ImageGroupOption>): ImageGroupOption | null => {
      if (typeof group.value !== 'string' || !Array.isArray(group.models)) {
        return null
      }
      const models = group.models
        .map((model: Partial<ImageModelOption>): ImageModelOption | null => {
          if (typeof model.value !== 'string') return null
          return {
            label: model.label || model.value,
            value: model.value,
            capabilities: normalizeImageCapabilities(model.capabilities),
          }
        })
        .filter(Boolean) as ImageModelOption[]
      if (models.length === 0) return null

      return {
        label: group.label || group.value,
        value: group.value,
        ratio: group.ratio ?? 1,
        desc: group.desc,
        models,
      }
    })
    .filter(Boolean) as ImageGroupOption[]
}

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
import type {
  ImageGenerationConfig,
  ImageGenerationRequest,
  ImageModelCapabilities,
  ImageReferenceInput,
} from '../types'
import { normalizePlaygroundImageConfig } from './image-generation-capabilities'

export function buildImageGenerationPayload(
  prompt: string,
  config: ImageGenerationConfig,
  capabilities: ImageModelCapabilities
): ImageGenerationRequest {
  const normalizedConfig = normalizePlaygroundImageConfig(config, capabilities)
  const payload: ImageGenerationRequest = {
    model: normalizedConfig.model,
    group: normalizedConfig.group,
    prompt: prompt.trim(),
    n: 1,
    response_format: normalizedConfig.response_format,
  }

  if (capabilities.size_mode === 'dimensions' && normalizedConfig.size) {
    payload.size = normalizedConfig.size
  }
  if (capabilities.size_mode === 'aspect_ratio_resolution') {
    payload.aspect_ratio = normalizedConfig.aspect_ratio
    payload.resolution = normalizedConfig.resolution
  }
  if (capabilities.qualities.length > 0) {
    payload.quality = normalizedConfig.quality
  }
  if (
    capabilities.output_formats.length > 0 &&
    normalizedConfig.output_format
  ) {
    payload.output_format = normalizedConfig.output_format
  }
  if (
    capabilities.supports_output_compression &&
    normalizedConfig.output_compression !== undefined &&
    normalizedConfig.output_compression !== null
  ) {
    payload.output_compression = normalizedConfig.output_compression
  }
  if (capabilities.supports_moderation && normalizedConfig.moderation) {
    payload.moderation = normalizedConfig.moderation
  }

  return payload
}

export function buildImageEditFormData(
  prompt: string,
  config: ImageGenerationConfig,
  referenceImages: ImageReferenceInput[],
  capabilities: ImageModelCapabilities
): FormData {
  const normalizedConfig = normalizePlaygroundImageConfig(config, capabilities)
  const formData = new FormData()

  formData.append('model', normalizedConfig.model)
  formData.append('group', normalizedConfig.group)
  formData.append('prompt', prompt.trim())
  formData.append('n', '1')
  formData.append('response_format', normalizedConfig.response_format)

  if (capabilities.size_mode === 'dimensions' && normalizedConfig.size) {
    formData.append('size', normalizedConfig.size)
  }
  if (capabilities.size_mode === 'aspect_ratio_resolution') {
    formData.append('aspect_ratio', normalizedConfig.aspect_ratio)
    formData.append('resolution', normalizedConfig.resolution)
  }
  if (capabilities.qualities.length > 0) {
    formData.append('quality', normalizedConfig.quality)
  }
  if (
    capabilities.output_formats.length > 0 &&
    normalizedConfig.output_format
  ) {
    formData.append('output_format', normalizedConfig.output_format)
  }
  if (
    capabilities.supports_output_compression &&
    normalizedConfig.output_compression !== undefined &&
    normalizedConfig.output_compression !== null
  ) {
    formData.append(
      'output_compression',
      String(normalizedConfig.output_compression)
    )
  }
  if (capabilities.supports_moderation && normalizedConfig.moderation) {
    formData.append('moderation', normalizedConfig.moderation)
  }

  referenceImages.forEach((reference) => {
    formData.append('image', reference.file, reference.file.name)
  })

  return formData
}

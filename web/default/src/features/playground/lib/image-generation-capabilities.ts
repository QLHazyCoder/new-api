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
  ImageGroupOption,
  ImageModelCapabilities,
  ImageModelOption,
} from '../types'

export const MAX_IMAGE_GENERATION_COUNT = 4

export const EMPTY_IMAGE_MODEL_CAPABILITIES: ImageModelCapabilities = {
  provider: 'other',
  size_mode: 'none',
  sizes: [],
  aspect_ratios: [],
  resolutions: [],
  qualities: [],
  output_formats: [],
  supports_editing: false,
  supports_moderation: false,
  supports_output_compression: false,
  max_images: 1,
}

export interface ImageModelSelection {
  group: ImageGroupOption
  model: ImageModelOption
}

function normalizeOption(
  value: string | undefined,
  options: string[],
  defaultValue?: string
): string {
  const selected = options.find(
    (option) => option.toLowerCase() === value?.toLowerCase()
  )
  if (selected) return selected

  const defaultOption = options.find(
    (option) => option.toLowerCase() === defaultValue?.toLowerCase()
  )
  return defaultOption || options[0] || ''
}

export function normalizePlaygroundImageConfig(
  config: ImageGenerationConfig,
  capabilities: ImageModelCapabilities
): ImageGenerationConfig {
  const size =
    capabilities.size_mode === 'dimensions'
      ? normalizeOption(
          config.size,
          capabilities.sizes,
          capabilities.default_size
        )
      : ''
  const aspectRatio =
    capabilities.size_mode === 'aspect_ratio_resolution'
      ? normalizeOption(
          config.aspect_ratio,
          capabilities.aspect_ratios,
          capabilities.default_aspect_ratio
        )
      : ''
  const resolution =
    capabilities.size_mode === 'aspect_ratio_resolution'
      ? normalizeOption(
          config.resolution,
          capabilities.resolutions,
          capabilities.default_resolution
        )
      : ''
  const quality = normalizeOption(
    config.quality,
    capabilities.qualities,
    capabilities.default_quality
  )
  const outputFormat = normalizeOption(
    config.output_format,
    capabilities.output_formats,
    capabilities.default_output_format
  )

  return {
    ...config,
    size,
    aspect_ratio: aspectRatio,
    resolution,
    quality: (quality || 'auto') as ImageGenerationConfig['quality'],
    n: normalizeImageGenerationCount(config.n, capabilities.max_images),
    response_format: 'b64_json',
    output_format:
      (outputFormat as ImageGenerationConfig['output_format']) || undefined,
  }
}

export function normalizeImageGenerationCount(
  count: number,
  modelMaximum: number = MAX_IMAGE_GENERATION_COUNT
): number {
  const maximum = Math.min(
    MAX_IMAGE_GENERATION_COUNT,
    Math.max(1, modelMaximum)
  )
  return Math.min(maximum, Math.max(1, Number.isFinite(count) ? count : 1))
}

export function resolveImageModelSelection(
  groups: ImageGroupOption[],
  groupValue: string,
  modelValue: string
): ImageModelSelection | null {
  if (groups.length === 0) return null
  const group =
    groups.find((option) => option.value === groupValue) ||
    groups.find((option) => option.value === 'default') ||
    groups[0]
  const model =
    group.models.find((option) => option.value === modelValue) ||
    group.models[0]
  if (!model) return null
  return { group, model }
}

export function findImageModelCapabilities(
  groups: ImageGroupOption[],
  config: Pick<ImageGenerationConfig, 'group' | 'model'>
): ImageModelCapabilities | null {
  const group = groups.find((option) => option.value === config.group)
  const model = group?.models.find((option) => option.value === config.model)
  return model?.capabilities || null
}

export function imageConfigsEqual(
  left: ImageGenerationConfig,
  right: ImageGenerationConfig
): boolean {
  return (
    left.model === right.model &&
    left.group === right.group &&
    left.size === right.size &&
    left.aspect_ratio === right.aspect_ratio &&
    left.resolution === right.resolution &&
    left.quality === right.quality &&
    left.n === right.n &&
    left.response_format === right.response_format &&
    left.output_format === right.output_format &&
    left.output_compression === right.output_compression &&
    left.moderation === right.moderation
  )
}

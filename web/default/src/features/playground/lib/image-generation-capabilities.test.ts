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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { DEFAULT_IMAGE_CONFIG } from '../constants'
import type { ImageGroupOption, ImageModelCapabilities } from '../types'
import {
  normalizePlaygroundImageConfig,
  resolveImageModelSelection,
} from './image-generation-capabilities'

const grokCapabilities: ImageModelCapabilities = {
  provider: 'xai',
  size_mode: 'aspect_ratio_resolution',
  sizes: [],
  aspect_ratios: ['auto', '16:9'],
  resolutions: ['1K', '2K'],
  qualities: [],
  output_formats: [],
  default_aspect_ratio: 'auto',
  default_resolution: '1K',
  supports_editing: false,
  supports_moderation: false,
  supports_output_compression: false,
  max_images: 4,
}

describe('playground image capabilities', () => {
  test('falls back atomically to a group that has an image model', () => {
    const groups: ImageGroupOption[] = [
      {
        label: 'Grok',
        value: 'Grok',
        ratio: 1,
        models: [
          {
            label: 'grok-imagine-image',
            value: 'grok-imagine-image',
            capabilities: grokCapabilities,
          },
        ],
      },
    ]

    const selection = resolveImageModelSelection(
      groups,
      'text-only-group',
      'text-only-model'
    )
    assert.equal(selection?.group.value, 'Grok')
    assert.equal(selection?.model.value, 'grok-imagine-image')
  })

  test('normalizes provider-specific controls without retaining GPT size', () => {
    const normalized = normalizePlaygroundImageConfig(
      {
        ...DEFAULT_IMAGE_CONFIG,
        model: 'grok-imagine-image',
        group: 'Grok',
        aspect_ratio: '16:9',
        resolution: '2K',
      },
      grokCapabilities
    )

    assert.equal(normalized.size, '')
    assert.equal(normalized.aspect_ratio, '16:9')
    assert.equal(normalized.resolution, '2K')
    assert.equal(normalized.output_format, undefined)
  })
})

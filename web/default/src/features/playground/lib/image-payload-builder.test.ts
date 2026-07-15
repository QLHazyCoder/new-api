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
import type { ImageModelCapabilities } from '../types'
import { buildImageGenerationPayload } from './image-payload-builder'

const gptCapabilities: ImageModelCapabilities = {
  provider: 'openai',
  size_mode: 'dimensions',
  sizes: ['1024x1024'],
  aspect_ratios: [],
  resolutions: [],
  qualities: ['auto', 'high'],
  output_formats: ['png', 'jpeg', 'webp'],
  default_size: '1024x1024',
  default_quality: 'auto',
  default_output_format: 'png',
  supports_editing: true,
  supports_moderation: true,
  supports_output_compression: true,
  max_images: 4,
}

const geminiCapabilities: ImageModelCapabilities = {
  provider: 'gemini',
  size_mode: 'aspect_ratio_resolution',
  sizes: [],
  aspect_ratios: ['1:1', '16:9'],
  resolutions: ['1K', '2K', '4K'],
  qualities: [],
  output_formats: [],
  default_aspect_ratio: '1:1',
  default_resolution: '1K',
  supports_editing: false,
  supports_moderation: false,
  supports_output_compression: false,
  max_images: 4,
}

const fixedGeminiCapabilities: ImageModelCapabilities = {
  ...geminiCapabilities,
  resolutions: [],
  default_resolution: '4K',
}

describe('image payload builder', () => {
  test('keeps GPT dimensions and output controls', () => {
    const payload = buildImageGenerationPayload(
      '  product photo  ',
      DEFAULT_IMAGE_CONFIG,
      gptCapabilities
    )

    assert.equal(payload.prompt, 'product photo')
    assert.equal(payload.size, '1024x1024')
    assert.equal(payload.quality, 'auto')
    assert.equal(payload.output_format, 'png')
    assert.equal(payload.aspect_ratio, undefined)
    assert.equal(payload.resolution, undefined)
    assert.equal(payload.response_format, 'b64_json')
  })

  test('sends Gemini aspect ratio and resolution without GPT-only fields', () => {
    const payload = buildImageGenerationPayload(
      'poster',
      {
        ...DEFAULT_IMAGE_CONFIG,
        model: 'gemini-3.1-flash-image',
        group: 'Gemini',
        aspect_ratio: '16:9',
        resolution: '4K',
      },
      geminiCapabilities
    )

    assert.equal(payload.aspect_ratio, '16:9')
    assert.equal(payload.resolution, '4K')
    assert.equal(payload.size, undefined)
    assert.equal(payload.quality, undefined)
    assert.equal(payload.output_format, undefined)
  })

  test('omits resolution when Gemini model name locks the image size', () => {
    const payload = buildImageGenerationPayload(
      'poster',
      {
        ...DEFAULT_IMAGE_CONFIG,
        model: 'gemini-3.1-flash-image-4K',
        group: 'Gemini',
        aspect_ratio: '16:9',
        resolution: '1K',
      },
      fixedGeminiCapabilities
    )

    assert.equal(payload.model, 'gemini-3.1-flash-image-4K')
    assert.equal(payload.aspect_ratio, '16:9')
    assert.equal(payload.resolution, undefined)
  })
})

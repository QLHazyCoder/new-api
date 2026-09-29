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
import { describe, expect, it } from 'vitest'

import type { PricingModel } from '../../types'
import {
  buildImageSample,
  buildVideoSample,
  imageSampleBody,
} from '../media-samples'
import { buildRateLimits, buildSupportedParameters } from '../mock-stats'

const imageCapabilities: NonNullable<PricingModel['image_capabilities']> = {
  provider: 'openai',
  size_mode: 'dimensions',
  sizes: ['1024x1024', '3840x2160'],
  qualities: ['auto', 'low', 'high'],
  output_formats: ['png', 'jpeg'],
  aspect_ratios: [],
  resolutions: [],
  default_size: '3840x2160',
  default_quality: 'auto',
  default_output_format: 'png',
  supports_editing: true,
  supports_moderation: true,
  supports_output_compression: false,
  max_images: 4,
}

const model: PricingModel = {
  id: 1,
  model_name: 'public-image',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 1,
  enable_groups: ['default'],
  supported_endpoint_types: ['image-generation'],
  image_capabilities: imageCapabilities,
}

const ctx = {
  baseUrl: 'https://api.example.com',
  endpointPath: '/v1/images/generations',
  modelName: 'public-image',
  apiKeyEnv: 'NEW_API_KEY',
}

describe('catalog media samples', () => {
  it('uses mapped image capabilities instead of a generic image template', () => {
    expect(imageSampleBody('public-image', imageCapabilities)).toMatchObject({
      model: 'public-image',
      size: '3840x2160',
      quality: 'auto',
      output_format: 'png',
      n: 1,
    })
    const params = buildSupportedParameters(model)
    expect(params.find((param) => param.name === 'size')?.enumValues).toEqual([
      '1024x1024',
      '3840x2160',
    ])
    expect(params.find((param) => param.name === 'n')?.range).toBe('1 ~ 4')
    expect(params.map((param) => param.name)).not.toContain('style')
    expect(buildImageSample('curl', ctx, imageCapabilities)).toContain(
      '"size": "3840x2160"'
    )
  })

  it('does not invent size, quality, or resolution without image capability data', () => {
    expect(imageSampleBody('unknown-image')).toEqual({
      model: 'unknown-image',
      prompt: 'A serene koi pond at sunset.',
    })
    expect(
      buildSupportedParameters({ ...model, image_capabilities: undefined })
    ).toHaveLength(2)
  })

  it('uses aspect-ratio options only when the configured capability supports them', () => {
    const capability: NonNullable<PricingModel['image_capabilities']> = {
      ...imageCapabilities,
      size_mode: 'aspect_ratio_resolution',
      sizes: [],
      aspect_ratios: ['1:1', '16:9'],
      resolutions: ['1K', '2K'],
      default_aspect_ratio: '16:9',
      default_resolution: '2K',
    }
    expect(imageSampleBody('grok-image', capability)).toMatchObject({
      aspect_ratio: '16:9',
      resolution: '2K',
    })
    expect(imageSampleBody('grok-image', capability)).not.toHaveProperty('size')
  })

  it('generates video create, poll, and content examples for all languages', () => {
    for (const lang of [
      'curl',
      'python',
      'typescript',
      'javascript',
    ] as const) {
      const sample = buildVideoSample(lang, {
        ...ctx,
        endpointPath: '/v1/videos',
        modelName: 'grok-imagine-video',
      })
      expect(sample).toContain('/v1/videos')
      expect(sample).toContain('/content')
      expect(sample).toContain('grok-imagine-video')
      expect(sample).not.toContain('/v1/chat/completions')
      expect(sample).not.toContain('"fps"')
      if (lang === 'typescript' || lang === 'javascript') {
        expect(sample).toContain("writeFile('video.mp4'")
      }
    }
    const video = {
      ...model,
      model_name: 'grok-imagine-video',
      supported_endpoint_types: ['openai-video'],
    }
    expect(buildSupportedParameters(video).map((param) => param.name)).toEqual([
      'model',
      'prompt',
    ])
  })

  it('does not infer a video endpoint from a misleading model name', () => {
    const onlyChat = {
      ...model,
      model_name: 'wan3.0-video',
      supported_endpoint_types: ['openai'],
    }
    expect(
      buildSupportedParameters(onlyChat).map((param) => param.name)
    ).toContain('temperature')
    expect(
      buildSupportedParameters(onlyChat).map((param) => param.name)
    ).not.toContain('fps')
  })

  it('keeps static rate limits unchanged when endpoint metadata changes', () => {
    const chat = {
      ...model,
      model_name: 'unclassified-model',
      supported_endpoint_types: ['openai'],
    }
    const video = { ...chat, supported_endpoint_types: ['openai-video'] }

    expect(buildRateLimits(video)).toEqual(buildRateLimits(chat))
  })
})

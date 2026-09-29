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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'

import { ModelDetailsApi } from '../components/model-details-api'
import type { PricingModel } from '../types'

const model: PricingModel = {
  id: 1,
  model_name: 'multi-media-model',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 1,
  enable_groups: ['default'],
  supported_endpoint_types: ['image-generation', 'openai-video'],
  image_capabilities: {
    provider: 'openai',
    size_mode: 'dimensions',
    sizes: ['1024x1024'],
    aspect_ratios: [],
    resolutions: [],
    qualities: [],
    output_formats: [],
    default_size: '1024x1024',
    default_aspect_ratio: '',
    default_resolution: '',
    default_quality: undefined,
    default_output_format: undefined,
    supports_editing: false,
    supports_moderation: false,
    supports_output_compression: false,
    max_images: 1,
  },
}

describe('model details API tab', () => {
  it('updates displayed request parameters when selecting the video endpoint', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(STATUS_QUERY_KEY, {
      server_address: 'https://api.example.com',
    })
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <ModelDetailsApi
          model={model}
          endpointMap={{
            'image-generation': { path: '/v1/images/generations' },
            'openai-video': { path: '/v1/videos' },
          }}
        />
      </QueryClientProvider>
    )

    const parameters = screen
      .getByText('Supported parameters')
      .closest('section')
    if (!parameters) throw new Error('Supported parameters section is missing')
    expect(within(parameters).getByText('size')).toBeVisible()

    await user.click(screen.getByRole('tab', { name: 'openai-video' }))

    expect(screen.getByRole('tab', { name: 'openai-video' })).toHaveAttribute(
      'data-active'
    )
    expect(within(parameters).getByText('prompt')).toBeVisible()
    expect(within(parameters).queryByText('size')).not.toBeInTheDocument()
    client.clear()
  })
})

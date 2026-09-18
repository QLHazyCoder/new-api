/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  buildCCSwitchURL,
  CCSWITCH_SOURCES,
  normalizeServerAddress,
} from '../cc-switch-sources'

function sourceFor(appId: string) {
  const source = CCSWITCH_SOURCES.find((item) => item.appId === appId)
  if (!source) throw new Error(`Missing CC Switch source: ${appId}`)
  return source
}

describe('CC Switch source registry', () => {
  test('contains the seven importable sources and the unsupported desktop entry', () => {
    expect(CCSWITCH_SOURCES.map((source) => source.appId)).toEqual([
      'claude',
      'claude-desktop',
      'codex',
      'gemini',
      'grokbuild',
      'opencode',
      'openclaw',
      'hermes',
    ])
    expect(CCSWITCH_SOURCES.filter((source) => source.supported)).toHaveLength(
      7
    )
    expect(
      new Set(CCSWITCH_SOURCES.map((source) => source.defaultName))
    ).toEqual(new Set(['coder']))

    const desktop = CCSWITCH_SOURCES.find(
      (source) => source.appId === 'claude-desktop'
    )
    expect(desktop).toMatchObject({
      supported: false,
      modelFields: [],
      unsupportedReasonKey: 'Claude Desktop setup instructions',
    })
  })

  test('normalizes trailing slashes and an existing v1 suffix', () => {
    expect(normalizeServerAddress(' https://example.com/v1/// ')).toBe(
      'https://example.com'
    )
    expect(normalizeServerAddress('https://example.com///')).toBe(
      'https://example.com'
    )
    expect(normalizeServerAddress('https://example.com/V1')).toBe(
      'https://example.com'
    )
    expect(normalizeServerAddress('https://example.com/v1/v1/')).toBe(
      'https://example.com'
    )
    expect(normalizeServerAddress('https://example.com/v1//v1')).toBe(
      'https://example.com'
    )
  })

  test('builds encoded links with source-specific endpoints and model fields', () => {
    const claude = sourceFor('claude')
    const claudeURL = buildCCSwitchURL(
      claude,
      'Relay & Claude',
      {
        model: ' claude-sonnet ',
        haikuModel: 'claude-haiku',
        sonnetModel: 'claude-sonnet',
        opusModel: 'claude-opus',
      },
      'sk-key with spaces',
      'https://api.example.com/v1///'
    )
    expect(claudeURL).not.toBeNull()
    if (!claudeURL) throw new Error('Expected a Claude deep link')
    expect(claudeURL).toContain('Relay+%26+Claude')
    expect(claudeURL).toContain('sk-key+with+spaces')
    const claudeParams = new URL(claudeURL).searchParams
    expect(claudeParams.get('app')).toBe('claude')
    expect(claudeParams.get('name')).toBe('Relay & Claude')
    expect(claudeParams.get('endpoint')).toBe('https://api.example.com')
    expect(claudeParams.get('homepage')).toBe('https://api.example.com')
    expect(claudeParams.get('apiKey')).toBe('sk-key with spaces')
    expect(claudeParams.get('model')).toBe('claude-sonnet')
    expect(claudeParams.get('haikuModel')).toBe('claude-haiku')
    expect(claudeParams.get('sonnetModel')).toBe('claude-sonnet')
    expect(claudeParams.get('opusModel')).toBe('claude-opus')
    expect(claudeParams.get('enabled')).toBe('true')

    for (const source of CCSWITCH_SOURCES.filter((item) => item.supported)) {
      const url = buildCCSwitchURL(
        source,
        source.defaultName,
        { model: 'model-a' },
        'sk-test',
        'https://api.example.com/v1/'
      )
      expect(url).not.toBeNull()
      if (!url) throw new Error(`Expected a deep link for ${source.appId}`)
      const params = new URL(url).searchParams
      const expectedEndpoint = `https://api.example.com${source.endpointSuffix}`
      expect(params.get('endpoint')).toBe(expectedEndpoint)
      expect(params.get('homepage')).toBe('https://api.example.com')
      expect(params.get('model')).toBe('model-a')
    }
  })

  test('does not create a deep link for Claude Desktop', () => {
    const desktop = sourceFor('claude-desktop')
    expect(
      buildCCSwitchURL(
        desktop,
        'Desktop',
        {},
        'sk-secret',
        'https://api.example.com'
      )
    ).toBeNull()
  })

  test('does not create a supported deep link without its required model', () => {
    const codex = sourceFor('codex')
    expect(
      buildCCSwitchURL(
        codex,
        'Codex',
        {},
        'sk-secret',
        'https://api.example.com'
      )
    ).toBeNull()
  })
})

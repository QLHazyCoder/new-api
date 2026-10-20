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
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import {
  isSecureDocsImageURL,
  normalizeStandaloneDocsImages,
} from '../markdown'
import { DocsMarkdown } from '../markdown-view'

describe('public documentation Markdown', () => {
  test('only supported standalone HTTPS image URLs are converted', () => {
    expect(
      normalizeStandaloneDocsImages(
        'https://assets.example/image.PNG?size=2#preview'
      )
    ).toBe('![](https://assets.example/image.PNG?size=2#preview)')
    expect(
      normalizeStandaloneDocsImages(
        '[https://assets.example/image.webp](https://assets.example/image.webp)'
      )
    ).toBe('![](https://assets.example/image.webp)')
    expect(normalizeStandaloneDocsImages('https://assets.example/page')).toBe(
      'https://assets.example/page'
    )
    expect(
      normalizeStandaloneDocsImages('http://assets.example/image.png')
    ).toBe('http://assets.example/image.png')
  })

  test('HTTPS explicit images allow extensionless URLs but reject other protocols', () => {
    expect(isSecureDocsImageURL('https://assets.example/image?id=2')).toBe(true)
    expect(isSecureDocsImageURL('http://assets.example/image.png')).toBe(false)
    expect(isSecureDocsImageURL('//assets.example/image.png')).toBe(false)
    expect(
      isSecureDocsImageURL('https://assets.example/image.png.exe', true)
    ).toBe(false)
  })

  test('GFM renders while raw HTML and non-HTTPS images do not create active elements', () => {
    const { container } = render(
      <DocsMarkdown
        markdown={[
          '| Name | Value |',
          '| --- | --- |',
          '| one | two |',
          '',
          '<script>alert(1)</script>',
          '',
          '![blocked](http://assets.example/image.png)',
        ].join('\n')}
      />
    )

    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    expect(screen.getByText('blocked')).toBeInTheDocument()
  })

  test('renders explicit, standalone, and self-linked HTTPS image forms', () => {
    const { container } = render(
      <DocsMarkdown
        markdown={[
          '![explicit](https://assets.example/without-extension)',
          '',
          'https://assets.example/standalone.png?size=2#preview',
          '',
          '[https://assets.example/linked.webp](https://assets.example/linked.webp)',
        ].join('\n')}
      />
    )

    expect(container.querySelectorAll('img')).toHaveLength(3)
  })

  test('standalone image URLs inside fenced code remain code text', () => {
    const markdown = '```text\nhttps://assets.example/image.png\n```'
    expect(normalizeStandaloneDocsImages(markdown)).toBe(markdown)
  })
})

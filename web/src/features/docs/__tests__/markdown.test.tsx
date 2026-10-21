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
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { Toaster } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  extractDocumentationHeadings,
  isSecureDocsImageURL,
  normalizeStandaloneDocsImages,
} from '../markdown'
import { DocsMarkdown } from '../markdown-view'

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

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

  test('extracts unique headings while ignoring fenced code and math blocks', () => {
    expect(
      extractDocumentationHeadings(
        [
          '## 快速开始',
          '### Install `the package`',
          '## 快速开始',
          '```md',
          '## Not a section',
          '```',
          '',
          '$$',
          '## Not a math section',
          '$$',
        ].join('\n')
      )
    ).toEqual([
      { id: '快速开始', depth: 2, text: '快速开始' },
      { id: 'install-the-package', depth: 3, text: 'Install the package' },
      { id: '快速开始-2', depth: 2, text: '快速开始' },
    ])
  })

  test('heading IDs match rendered nested headings and do not collide with existing suffixes', () => {
    const markdown = [
      '# Overview',
      '> ## **Install** `the package`',
      '',
      '## Install the package-2',
      '## Install the package',
      '',
      '使用说明 &amp; API',
      '---',
    ].join('\n')
    const headings = extractDocumentationHeadings(markdown)
    render(<DocsMarkdown markdown={markdown} headings={headings} />)

    expect(headings).toEqual([
      { id: 'overview', depth: 1, text: 'Overview' },
      { id: 'install-the-package', depth: 2, text: 'Install the package' },
      { id: 'install-the-package-2', depth: 2, text: 'Install the package-2' },
      { id: 'install-the-package-3', depth: 2, text: 'Install the package' },
      { id: '使用说明-api', depth: 2, text: '使用说明 & API' },
    ])
    expect(screen.getAllByRole('heading').map((heading) => heading.id)).toEqual(
      headings.map((heading) => heading.id)
    )
  })

  test('renders safe Markdown links and keeps code URLs as code', () => {
    const { container } = render(
      <DocsMarkdown
        markdown={[
          '[API Reference](https://example.com/api?version=1#models)',
          '',
          'https://example.com/visible',
          '',
          '[Unsafe](javascript:alert%281%29)',
          '',
          '```text',
          'https://example.com/code',
          '```',
        ].join('\n')}
      />
    )

    expect(screen.getByRole('link', { name: 'API Reference' })).toHaveAttribute(
      'href',
      'https://example.com/api?version=1#models'
    )
    expect(screen.getByRole('link', { name: 'API Reference' })).toHaveAttribute(
      'target',
      '_blank'
    )
    expect(screen.getByRole('link', { name: 'API Reference' })).toHaveAttribute(
      'rel',
      'noopener noreferrer'
    )
    expect(
      screen.getByRole('link', { name: 'https://example.com/visible' })
    ).toHaveAttribute('href', 'https://example.com/visible')
    expect(
      screen.queryByRole('link', { name: 'Unsafe' })
    ).not.toBeInTheDocument()
    expect(container.querySelector('pre code')).toHaveTextContent(
      'https://example.com/code'
    )
  })

  test.each([
    ['HTTP help', 'http://example.com/help'],
    ['Contact', 'mailto:help@example.com'],
  ])('renders %s as a clickable link', (label, href) => {
    render(<DocsMarkdown markdown={`[${label}](${href})`} />)
    expect(screen.getByRole('link', { name: label })).toHaveAttribute(
      'href',
      href
    )
  })

  test.each([
    'javascript:alert%281%29',
    'vbscript:msgbox%281%29',
    'data:text/html,test',
  ])('does not allow the dangerous link %s', (href) => {
    render(<DocsMarkdown markdown={`[Unsafe](${href})`} />)
    expect(screen.getByText('Unsafe')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  test('copies only fenced code text when its copy control is activated', async () => {
    const user = userEvent.setup()
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockResolvedValue(undefined)
    const code = '  printf "hello & <world>"\n\n  echo done  '
    render(
      <DocsMarkdown
        markdown={`Inline \`value\`.\n\n\`\`\`bash\n${code}\n\`\`\``}
      />
    )

    const copyButton = screen.getByRole('button', { name: 'Copy code' })
    await user.click(copyButton)

    expect(writeText).toHaveBeenCalledWith(code)
    expect(
      await screen.findByRole('button', { name: 'Copied!' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Copy code' })
    ).not.toBeInTheDocument()
  })

  test('resets the copied state after two seconds', async () => {
    userEvent.setup()
    vi.useFakeTimers()
    vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    render(<DocsMarkdown markdown={'```text\nexample\n```'} />)

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Copy code' }))
    })
    expect(screen.getByRole('button', { name: 'Copied!' })).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(2000))
    expect(
      screen.getByRole('button', { name: 'Copy code' })
    ).toBeInTheDocument()
  })

  test('switching the copy control language preserves the observed heading elements', async () => {
    render(
      <DocsMarkdown markdown={'## Installation\n\n```text\nexample\n```'} />
    )
    const section = screen.getByRole('heading', { name: 'Installation' })
    i18next.addResourceBundle('test-docs', 'translation', {
      'Copy code': '复制代码',
    })
    try {
      await act(async () => {
        await i18next.changeLanguage('test-docs')
      })
      expect(
        screen.getByRole('button', { name: '复制代码' })
      ).toBeInTheDocument()
      expect(screen.getByRole('heading', { name: 'Installation' })).toBe(
        section
      )
    } finally {
      await act(async () => {
        await i18next.changeLanguage('en')
      })
      i18next.removeResourceBundle('test-docs', 'translation')
    }
  })

  test('a failed copy shows an error and does not display success', async () => {
    const user = userEvent.setup()
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
      new Error('Clipboard denied')
    )
    vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    render(
      <>
        <DocsMarkdown markdown={'```text\nexample\n```'} />
        <Toaster />
      </>
    )

    await user.click(screen.getByRole('button', { name: 'Copy code' }))
    expect(
      await screen.findByText('Failed to copy to clipboard')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy code' })).toBeEnabled()
    expect(
      screen.queryByRole('button', { name: 'Copied!' })
    ).not.toBeInTheDocument()
  })

  test('does not add copy buttons to inline code, indented code, math or diagrams', () => {
    render(
      <DocsMarkdown
        markdown={[
          'Inline `value`.',
          '',
          '    indented code',
          '',
          '```math',
          '\\frac{1}{2}',
          '```',
          '',
          '```flow',
          'st=>start: Start',
          '```',
          '',
          '```seq',
          'Alice->Bob: Hello',
          '```',
        ].join('\n')}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Copy code' })
    ).not.toBeInTheDocument()
  })
})

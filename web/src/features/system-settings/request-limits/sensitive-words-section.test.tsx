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
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    get: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}))

vi.mock('@/lib/api', () => ({ api: apiMock }))

import { SensitiveWordsSection } from './sensitive-words-section'

const defaultValues = {
  CheckSensitiveEnabled: true,
  CheckSensitiveOnPromptEnabled: true,
  SensitiveWords: '',
}

const COMPONENT_TEST_TIMEOUT_MS = 30_000

function configureApiMocks() {
  apiMock.get.mockImplementation((path: string) => {
    if (path === '/api/sensitive-words/config') {
      return Promise.resolve({
        data: {
          data: {
            enabled: true,
            check_prompt: true,
            mode: 'block',
            audit_enabled: true,
            block_message: 'blocked',
            ban_threshold: 5,
            full_prompt_retention_days: 180,
            max_prompt_runes: 65536,
            rule_version: 1,
          },
        },
      })
    }
    if (path === '/api/sensitive-words/rules') {
      return Promise.resolve({ data: { data: [] } })
    }
    if (path === '/api/sensitive-words/groups') {
      return Promise.resolve({ data: { data: [] } })
    }
    throw new Error(`Unexpected GET ${path}`)
  })
  apiMock.put.mockResolvedValue({ data: { data: {} } })
  apiMock.post.mockResolvedValue({ data: { data: {} } })
  apiMock.patch.mockResolvedValue({ data: { data: {} } })
  apiMock.delete.mockResolvedValue({ data: { data: {} } })
}

async function openCreateDialog() {
  render(<SensitiveWordsSection defaultValues={defaultValues} />)
  await waitFor(() =>
    expect(screen.getByText('尚未创建敏感词规则')).toBeInTheDocument()
  )
  await userEvent.click(screen.getByRole('button', { name: '添加敏感词' }))
  return {
    search: screen.getByRole('textbox', {
      name: 'Search current words...',
    }) as HTMLInputElement,
    textarea: screen.getByLabelText('敏感词条') as HTMLTextAreaElement,
  }
}

describe('SensitiveWordsSection word search', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    configureApiMocks()
  })

  test(
    'selects the first match and cycles with Enter and Shift+Enter',
    async () => {
      const { search, textarea } = await openCreateDialog()
      const user = userEvent.setup()

      fireEvent.change(textarea, {
        target: { value: 'Alpha\nbeta Alpha' },
      })
      await user.type(search, 'alpha')

      await waitFor(() => {
        expect(screen.getByRole('status')).toHaveTextContent('1/2')
        expect(textarea.selectionStart).toBe(0)
        expect(textarea.selectionEnd).toBe(5)
      })

      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(screen.getByRole('status')).toHaveTextContent('2/2')
        expect(textarea.selectionStart).toBe(11)
        expect(textarea.selectionEnd).toBe(16)
      })

      await user.keyboard('{Shift>}{Enter}{/Shift}')
      await waitFor(() => {
        expect(screen.getByRole('status')).toHaveTextContent('1/2')
        expect(textarea.selectionStart).toBe(0)
        expect(textarea.selectionEnd).toBe(5)
      })
    },
    COMPONENT_TEST_TIMEOUT_MS
  )

  test(
    'recomputes after edits and TXT import without changing the draft',
    async () => {
      const { search, textarea } = await openCreateDialog()
      const user = userEvent.setup()

      fireEvent.change(textarea, { target: { value: 'first\nsecond' } })
      await user.type(search, 'alpha')
      fireEvent.blur(search)
      expect(search).toHaveValue('alpha')
      expect(screen.getByRole('status')).toHaveTextContent('0/0')

      textarea.focus()
      fireEvent.change(textarea, {
        target: { value: 'first\nAlpha\nsecond' },
      })
      expect(document.activeElement).toBe(textarea)
      await waitFor(() =>
        expect(screen.getByRole('status')).toHaveTextContent('1/1')
      )

      const fileInput = document.querySelector(
        'input[type="file"]'
      ) as HTMLInputElement
      const file = new File(['third\nALPHA'], 'words.txt', {
        type: 'text/plain',
      })
      fireEvent.change(fileInput, { target: { files: [file] } })

      await waitFor(() => {
        expect(textarea.value).toBe('first\nAlpha\nsecond\nthird\nALPHA')
        expect(screen.getByRole('status')).toHaveTextContent('1/2')
      })

      await user.click(screen.getByRole('button', { name: 'Clear search' }))
      expect(search).toHaveValue('')
      expect(screen.getByRole('status')).toHaveTextContent('0/0')
      expect(textarea.value).toBe('first\nAlpha\nsecond\nthird\nALPHA')

      await user.type(screen.getByLabelText('规则名称'), '保存测试')
      await user.click(screen.getByRole('button', { name: '保存规则' }))
      await waitFor(() => {
        expect(apiMock.post).toHaveBeenCalledWith(
          '/api/sensitive-words/rules',
          expect.objectContaining({
            name: '保存测试',
            words: ['first', 'Alpha', 'second', 'third'],
          })
        )
      })
    },
    COMPONENT_TEST_TIMEOUT_MS
  )

  test(
    'clears search state when the rule dialog is closed and reopened',
    async () => {
      const { search } = await openCreateDialog()
      const user = userEvent.setup()

      await user.type(search, 'alpha')
      expect(search).toHaveValue('alpha')
      await user.click(screen.getByRole('button', { name: '取消' }))
      await user.click(screen.getByRole('button', { name: '添加敏感词' }))

      expect(
        screen.getByRole('textbox', { name: 'Search current words...' })
      ).toHaveValue('')
    },
    COMPONENT_TEST_TIMEOUT_MS
  )
})

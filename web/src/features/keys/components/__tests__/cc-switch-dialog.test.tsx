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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

vi.mock('@/lib/api', () => ({
  getUserModels: vi.fn(async () => ({
    success: true,
    data: ['model-a', 'model-b'],
  })),
}))

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { CCSwitchDialog } = await import('../dialogs/cc-switch-dialog')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Application: 'Application',
        Cancel: 'Cancel',
        'CC Switch temporarily unavailable':
          'CC Switch temporarily unavailable',
        'Claude Code': 'Claude Code',
        'Claude Desktop': 'Claude Desktop',
        'Claude Desktop setup instructions':
          '用户 CC Switch 暂不支持，可在群内或者私聊 QQ：3834483010 机器人发送“Claude Desktop 配置流程”，即可获取方法',
        Codex: 'Codex',
        'Grok Build': 'Grok Build',
        Gemini: 'Gemini',
        Hermes: 'Hermes',
        'Import to CC Switch': 'Import to CC Switch',
        Name: 'Name',
        'No models found': 'No models found',
        'Open CC Switch': 'Open CC Switch',
        OpenClaw: 'OpenClaw',
        OpenCode: 'OpenCode',
        'Primary Model': 'Primary Model',
        'Select or enter model name': 'Select or enter model name',
        'Haiku Model': 'Haiku Model',
        'Sonnet Model': 'Sonnet Model',
        'Opus Model': 'Opus Model',
      },
    },
  },
})

afterEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
})

function renderDialog(apiKey?: {
  model_limits_enabled: boolean
  model_limits: string
}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <CCSwitchDialog
          open
          onOpenChange={() => undefined}
          tokenKey='token-value'
          apiKey={apiKey}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('CC Switch dialog', () => {
  test('shows the Claude Desktop instructions and prevents opening a link', async () => {
    renderDialog()
    await waitFor(() =>
      expect(
        screen.getByRole('radio', { name: 'Claude Desktop' })
      ).toBeInTheDocument()
    )

    fireEvent.click(screen.getByRole('radio', { name: 'Claude Desktop' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '用户 CC Switch 暂不支持，可在群内或者私聊 QQ：3834483010 机器人发送“Claude Desktop 配置流程”，即可获取方法'
    )
    expect(screen.queryByText('Primary Model')).not.toBeInTheDocument()
    expect(screen.queryByText('Name')).not.toBeInTheDocument()

    const unavailableButton = screen.getByRole('button', {
      name: 'CC Switch temporarily unavailable',
    })
    expect(unavailableButton).toBeDisabled()

    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)
    fireEvent.click(unavailableButton)
    expect(openSpy).not.toHaveBeenCalled()
  }, 15_000)

  test('resets the name and model fields when switching sources', async () => {
    renderDialog()
    await waitFor(() =>
      expect(screen.getByDisplayValue('coder')).toBeInTheDocument()
    )

    const [modelInput] = screen.getAllByPlaceholderText(
      'Select or enter model name'
    )
    if (!modelInput) throw new Error('Expected a Claude primary model input')
    fireEvent.input(modelInput, { target: { value: 'custom-model' } })
    expect(modelInput).toHaveValue('custom-model')

    fireEvent.click(screen.getByRole('radio', { name: 'Codex' }))
    expect(screen.getByDisplayValue('coder')).toBeInTheDocument()
    expect(
      screen.getByPlaceholderText('Select or enter model name')
    ).toHaveValue('')

    fireEvent.click(screen.getByRole('radio', { name: 'Claude Desktop' }))
    expect(screen.getByRole('alert')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: 'Claude Code' }))
    expect(screen.getByDisplayValue('coder')).toBeInTheDocument()
    expect(
      screen.getAllByPlaceholderText('Select or enter model name')[0]
    ).toHaveValue('')
  }, 15_000)

  test('shows only models configured for the current API key', async () => {
    renderDialog({
      model_limits_enabled: true,
      model_limits: 'key-only-model, model-b',
    })

    const [modelInput] = await waitFor(() => {
      const inputs = screen.getAllByPlaceholderText(
        'Select or enter model name'
      )
      expect(inputs.length).toBeGreaterThan(0)
      return inputs
    })
    const modelInputGroup = modelInput.parentElement
    if (!modelInputGroup) throw new Error('Expected a model input group')
    fireEvent.click(within(modelInputGroup).getByRole('button'))

    expect(await screen.findByText('key-only-model')).toBeInTheDocument()
    expect(screen.getByText('model-b')).toBeInTheDocument()
    expect(screen.queryByText('model-a')).not.toBeInTheDocument()
  }, 15_000)
})

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { SensitiveWordRulesTable } from './rules-table'
import type { SensitiveWordRuleDetail, SensitiveWordRuleSummary } from './types'

const rule: SensitiveWordRuleSummary = {
  id: 1,
  name: '规则测试',
  scope: 'group',
  mode: 'observe',
  groups: ['default'],
  word_count: 1,
  created_by: 1,
  version: 1,
  created_at: '2026-09-29T00:00:00Z',
  updated_at: '2026-09-29T00:00:00Z',
}

const detail: SensitiveWordRuleDetail = {
  ...rule,
  words: ['测试词'],
}

test('renders localized selected values in the table and editor', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'zh-CN',
    fallbackLng: 'en',
    resources: { 'zh-CN': zh, en },
    keySeparator: false,
    interpolation: { escapeValue: false },
  })
  const user = userEvent.setup()

  render(
    <I18nextProvider i18n={i18n}>
      <TooltipProvider>
        <SensitiveWordRulesTable
          rules={[rule]}
          groups={['default']}
          onReload={vi.fn().mockResolvedValue(undefined)}
          loadRule={vi.fn().mockResolvedValue(detail)}
        />
      </TooltipProvider>
    </I18nextProvider>
  )

  expect(screen.getByRole('combobox')).toHaveTextContent('观察')
  expect(screen.getByText('指定分组')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: '编辑规则' }))
  const dialog = await screen.findByRole('dialog')
  const dialogComboboxes = within(dialog).getAllByRole('combobox')
  expect(dialogComboboxes[0]).toHaveTextContent('观察')
  expect(dialogComboboxes[1]).toHaveTextContent('指定分组')
})

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
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { TieredPricingEditor } from '../tiered-pricing-editor'
import { getInitialTieredPricingEditorState } from '../tiered-pricing-editor-state'

const complexExpression =
  '(len <= 272000 ? tier("short_context", p * 5 + cr * 0.5 + c * 30) : tier("long_context", p * 10 + cr * 1 + c * 45)) * ((param("service_tier") == "fast" || param("service_tier") == "priority") ? 2.5 : 1)'

describe('tiered pricing editor initialization', () => {
  test('keeps complex expressions in raw mode without generating a zero expression', () => {
    const state = getInitialTieredPricingEditorState(complexExpression, '')

    expect(state.editorMode).toBe('raw')
    expect(state.visualConfig).toBeNull()
    expect(state.rawExpr).toBe(complexExpression)
    expect(state.visualConfigDirty).toBe(false)
  })

  test('does not write a complex expression back during the first render', () => {
    const onBillingExprChange = vi.fn()

    render(
      <TieredPricingEditor
        modelName='gpt-5.5'
        billingExpr={complexExpression}
        requestRuleExpr=''
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={vi.fn()}
      />
    )

    expect(onBillingExprChange).not.toHaveBeenCalled()
    expect(screen.getByRole('textbox')).toHaveValue(complexExpression)
    expect(screen.queryByDisplayValue('p * 0 + c * 0')).not.toBeInTheDocument()
  })

  test('keeps a new expression model empty until the user edits it', () => {
    const state = getInitialTieredPricingEditorState('', '')
    const onBillingExprChange = vi.fn()

    render(
      <TieredPricingEditor
        modelName='new-model'
        billingExpr=''
        requestRuleExpr=''
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={vi.fn()}
      />
    )

    expect(state.rawExpr).toBe('')
    expect(state.visualConfigDirty).toBe(false)
    expect(onBillingExprChange).not.toHaveBeenCalled()
  })
})

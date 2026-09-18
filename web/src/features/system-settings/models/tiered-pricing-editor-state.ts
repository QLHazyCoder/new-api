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
  combineBillingExpr,
  tryParseRequestRuleExpr,
  type RequestRuleGroup,
} from '@/features/pricing/lib/billing-expr'
import {
  createDefaultVisualConfig,
  tryParseVisualConfig,
  type VisualConfig,
} from '@/features/pricing/lib/tier-expr'

export type EditorMode = 'visual' | 'raw'

export type TieredPricingEditorState = {
  editorMode: EditorMode
  visualConfig: VisualConfig | null
  visualConfigDirty: boolean
  rawExpr: string
  requestRuleGroups: RequestRuleGroup[]
}

export function getInitialTieredPricingEditorState(
  currentExpr: string,
  currentRequestRuleExpr: string
): TieredPricingEditorState {
  const parsedConfig = tryParseVisualConfig(currentExpr)
  const parsedRuleGroups = tryParseRequestRuleExpr(currentRequestRuleExpr)
  const hasExpression = Boolean(currentExpr.trim())
  const canUseVisualMode = Boolean(parsedConfig) && parsedRuleGroups !== null

  return {
    editorMode:
      canUseVisualMode || (!hasExpression && !currentRequestRuleExpr.trim())
        ? 'visual'
        : 'raw',
    visualConfig: parsedConfig ??
      (hasExpression ? null : createDefaultVisualConfig()),
    visualConfigDirty: false,
    rawExpr: combineBillingExpr(currentExpr, currentRequestRuleExpr),
    requestRuleGroups: parsedRuleGroups || [],
  }
}

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
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import type { LogOtherData, SensitiveWordAuditEvent } from '../../types'
import { DetailsDialog } from '../dialogs/details-dialog'

const { getMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
}))

vi.mock('@/lib/api', () => ({ api: { get: getMock } }))

function makeLog(other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 7,
    created_at: 1,
    type: 8,
    content: '关键词拦截记录',
    username: 'audit-user',
    token_name: 'token',
    model_name: 'gpt-test',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: 'channel',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-sensitive',
    upstream_request_id: '',
  }
}

function renderDetails(log: UsageLog) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], {}, { updatedAt: Date.now() + 60_000 })

  render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={log}
        isAdmin
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )

  return queryClient
}

describe('sensitive-word usage log details', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  test('loads evidence from the top-level audit id and renders the request body', async () => {
    const event: SensitiveWordAuditEvent = {
      id: 42,
      request_id: 'req-sensitive',
      user_id: 7,
      username_snapshot: 'audit-user',
      group_name: 'default',
      model_name: 'gpt-test',
      endpoint: '/v1/responses',
      protocol: 'openai_responses',
      prompt_hash: 'hash-42',
      redacted_preview: '请执行 [REDACTED] 操作',
      full_prompt: '这是完整请求体\n危险词',
      matched_rule_ids: '[12]',
      matched_rule_names: '["全局规则"]',
      matched_words: '["危险词"]',
      matched_snippets: '["完整请求体\n危险词"]',
      matched_scope: 'global',
      blocked: true,
      violation_count: 3,
      auto_banned: false,
      observe_only: false,
      whitelist_bypassed: false,
      rule_version: 8,
    }
    getMock.mockResolvedValue({ data: { data: event } })

    const queryClient = renderDetails(
      makeLog({
        action: 'sensitive_word_block',
        audit_id: 42,
        keyword_filter: {
          action: 'blocked',
          blocked: true,
          request_id: 'req-sensitive',
          rule_ids: [12],
          rule_names: ['全局规则'],
          matched_words: ['危险词'],
        },
      })
    )

    await waitFor(() =>
      expect(getMock).toHaveBeenCalledWith('/api/log/sensitive-word-audit/42')
    )
    expect(await screen.findByText(/这是完整请求体/)).toBeInTheDocument()
    expect(screen.getAllByText('危险词').length).toBeGreaterThan(0)
    expect(screen.getByText('hash-42')).toBeInTheDocument()
    expect(screen.getByText('/v1/responses')).toBeInTheDocument()

    queryClient.clear()
  })
})

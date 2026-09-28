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
import { Check, Copy, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { getSensitiveWordAudit } from '../../api'
import type { UsageLog } from '../../data/schema'
import { getSensitiveWordOutcome } from '../../lib/sensitive-word-outcome'
import type { LogOtherData, SensitiveWordAuditEvent } from '../../types'
import { DetailRow, DetailSection } from './log-detail-layout'

function parseSensitiveAuditList(value: string | undefined): string[] {
  if (!value) return []
  try {
    const parsed = JSON.parse(value) as unknown
    return Array.isArray(parsed)
      ? parsed.filter((item): item is string => typeof item === 'string')
      : []
  } catch {
    return []
  }
}

export function SensitiveWordAuditSection(props: {
  auditId?: number
  log: UsageLog
  other: LogOtherData | null
  isAdmin: boolean
  open: boolean
  copiedText: string | null
  onCopy: (text: string) => void
}) {
  const { t } = useTranslation()
  const [event, setEvent] = useState<SensitiveWordAuditEvent | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    setEvent(null)
    setLoadFailed(false)
    if (!props.open || !props.isAdmin || !props.auditId) return
    void getSensitiveWordAudit(props.auditId)
      .then((response) => {
        if (!cancelled) {
          if (response.data) setEvent(response.data)
          else setLoadFailed(true)
        }
      })
      .catch(() => {
        if (!cancelled) setLoadFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [props.auditId, props.isAdmin, props.open])

  if (!props.isAdmin) return null

  const filter = props.other?.admin_info?.keyword_filter
  const outcome = getSensitiveWordOutcome(filter?.action)
  const matchedWords = parseSensitiveAuditList(event?.matched_words)
  const matchedRuleIds = parseSensitiveAuditList(event?.matched_rule_ids)
  const matchedRuleNames = parseSensitiveAuditList(event?.matched_rule_names)
  const matchedSnippets = parseSensitiveAuditList(event?.matched_snippets)
  const booleanLabel = (value: boolean | undefined) => {
    if (value === undefined) return '-'
    return t(value ? 'Yes' : 'No')
  }

  return (
    <>
      <DetailSection
        icon={<ShieldCheck className='size-3.5' aria-hidden='true' />}
        iconTone={outcome.blocked ? 'destructive' : 'info'}
        variant={outcome.blocked ? 'danger' : 'default'}
        label={t('Sensitive word audit')}
      >
        <DetailRow label={t('Result')} value={t(outcome.label)} />
        <DetailRow
          label={t('Whitelist bypassed')}
          value={booleanLabel(filter?.whitelist_bypassed)}
        />
        <DetailRow
          label={t('Observed')}
          value={booleanLabel(filter?.observe_only)}
        />
        <DetailRow
          label={t('Request ID')}
          value={props.log.request_id || '-'}
          mono
        />
        <DetailRow label={t('Group')} value={props.log.group || '-'} mono />
        <DetailRow
          label={t('Model')}
          value={props.log.model_name || '-'}
          mono
        />
        {event?.username_snapshot && (
          <DetailRow
            label={t('User')}
            value={`${event.username_snapshot} (#${event.user_id ?? '-'})`}
          />
        )}
        {event?.endpoint && (
          <DetailRow label={t('Endpoint')} value={event.endpoint} mono />
        )}
        {event?.protocol && (
          <DetailRow label={t('Protocol')} value={event.protocol} mono />
        )}
        <DetailRow
          label={t('Violation Count')}
          value={
            filter?.violation_count == null
              ? '-'
              : String(filter.violation_count)
          }
          mono
        />
        <DetailRow
          label={t('Automatic Ban')}
          value={booleanLabel(filter?.auto_banned)}
        />
        {event && (
          <DetailRow
            label={t('Rule version')}
            value={String(event.rule_version ?? '-')}
            mono
          />
        )}
        {(loadFailed || !props.auditId) && (
          <DetailRow
            label={t('Evidence')}
            value={t('Unable to load evidence')}
          />
        )}
        {!event && !loadFailed && props.auditId && (
          <DetailRow label={t('Evidence')} value={t('Loading...')} />
        )}
      </DetailSection>

      {event && (
        <DetailSection label={t('Rules')}>
          <DetailRow
            label={t('Rule IDs')}
            value={matchedRuleIds.length > 0 ? matchedRuleIds.join(', ') : '-'}
            mono
          />
          <DetailRow
            label={t('Rules')}
            value={
              matchedRuleNames.length > 0 ? matchedRuleNames.join(', ') : '-'
            }
          />
          <DetailRow
            label={t('Matched')}
            value={matchedWords.length > 0 ? matchedWords.join(', ') : '-'}
          />
          {matchedSnippets.length > 0 && (
            <div className='space-y-1'>
              <Label className='text-xs font-semibold'>
                {t('Matched snippets')}
              </Label>
              <pre className='bg-background/60 max-h-32 overflow-y-auto rounded border p-2 text-xs leading-relaxed whitespace-pre-wrap'>
                {matchedSnippets.join('\n')}
              </pre>
            </div>
          )}
        </DetailSection>
      )}

      {event && (
        <DetailSection label={t('Evidence')}>
          <DetailRow
            label={t('Prompt hash')}
            value={event.prompt_hash || '-'}
            mono
          />
          {event?.redacted_preview && (
            <div className='space-y-1'>
              <Label className='text-xs font-semibold'>
                {t('Redacted preview')}
              </Label>
              <pre className='bg-background/60 max-h-32 overflow-y-auto rounded border p-2 text-xs leading-relaxed whitespace-pre-wrap'>
                {event.redacted_preview}
              </pre>
            </div>
          )}
          <div className='bg-background/60 relative min-w-0 rounded-md border p-2'>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    className='absolute top-1 right-1'
                    onClick={() => props.onCopy(event?.full_prompt || '')}
                    disabled={!event?.full_prompt}
                    aria-label={t('Copy to clipboard')}
                  />
                }
              >
                {props.copiedText === event?.full_prompt ? (
                  <Check className='size-3 text-green-600' />
                ) : (
                  <Copy className='size-3' />
                )}
              </TooltipTrigger>
              <TooltipContent>{t('Copy to clipboard')}</TooltipContent>
            </Tooltip>
            <Label className='mb-1 block text-xs font-semibold'>
              {t('Full prompt')}
            </Label>
            <pre className='max-h-72 overflow-y-auto pr-6 font-mono text-xs leading-relaxed whitespace-pre-wrap'>
              {event?.full_prompt || t('Not retained or expired')}
            </pre>
          </div>
        </DetailSection>
      )}
    </>
  )
}

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
import {
  FileUp,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type KeyboardEvent,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { api } from '@/lib/api'

import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import {
  findSensitiveWordMatches,
  getNextSensitiveWordMatchIndex,
  type SensitiveWordTextMatch,
} from './sensitive-word-search'

const DEFAULT_BLOCK_MESSAGE =
  '你的请求因命中敏感词已被拦截，已记录 1 次；累计达到 {{threshold}} 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。'

type SensitiveWordPolicy = {
  enabled: boolean
  check_prompt: boolean
  retain_full_prompt: boolean
  block_message: string
  ban_threshold: number
  full_prompt_retention_days: number
  max_prompt_runes: number
  version: number
}

type RuleSummary = {
  id: number
  name: string
  scope: 'global' | 'group'
  groups: string[]
  word_count: number
  mode: 'block' | 'observe' | 'off'
  created_by: number
  version: number
  created_at: string
  updated_at: string
}

type RuleDetail = RuleSummary & {
  words: string[]
}

type RuleDraft = {
  id?: number
  name: string
  wordsText: string
  scope: 'global' | 'group'
  groups: string[]
  mode: 'block' | 'observe' | 'off'
}

type ParsedWords = {
  words: string[]
  blankCount: number
  duplicateCount: number
  tooLongCount: number
}

const emptyDraft = (): RuleDraft => ({
  name: '',
  wordsText: '',
  scope: 'global',
  groups: [],
  mode: 'observe',
})

function parseWords(value: string): ParsedWords {
  const words: string[] = []
  const seen = new Set<string>()
  let blankCount = 0
  let duplicateCount = 0
  let tooLongCount = 0

  for (const raw of value.split(/\r?\n/)) {
    const word = raw.trim()
    if (!word) {
      blankCount += 1
      continue
    }
    if ([...word].length > 200) {
      tooLongCount += 1
      continue
    }
    const normalized = word.toLocaleLowerCase()
    if (seen.has(normalized)) {
      duplicateCount += 1
      continue
    }
    seen.add(normalized)
    words.push(word)
  }

  return { words, blankCount, duplicateCount, tooLongCount }
}

function getErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function formatTime(value: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function createDefaultSensitiveWordConfig(): SensitiveWordPolicy {
  return {
    enabled: false,
    check_prompt: false,
    retain_full_prompt: true,
    block_message: DEFAULT_BLOCK_MESSAGE,
    ban_threshold: 50,
    full_prompt_retention_days: 180,
    max_prompt_runes: 65536,
    version: 1,
  }
}

export function SensitiveWordsSection() {
  const { t } = useTranslation()
  const defaultConfigRef = useRef<SensitiveWordPolicy>(
    createDefaultSensitiveWordConfig()
  )
  const [config, setConfig] = useState<SensitiveWordPolicy>(
    () => defaultConfigRef.current
  )
  const [rules, setRules] = useState<RuleSummary[]>([])
  const [groups, setGroups] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSavingConfig, setIsSavingConfig] = useState(false)
  const [isSavingRule, setIsSavingRule] = useState(false)
  const [changingModeRuleID, setChangingModeRuleID] = useState<number | null>(
    null
  )
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false)
  const [draft, setDraft] = useState<RuleDraft>(emptyDraft)
  const [deleteTarget, setDeleteTarget] = useState<RuleSummary | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const [wordSearch, setWordSearch] = useState('')
  const [activeMatchIndex, setActiveMatchIndex] = useState(0)
  const initialConfigRef = useRef<SensitiveWordPolicy | null>(null)

  const reload = useCallback(async () => {
    setIsLoading(true)
    try {
      const [configRes, rulesRes, groupsRes] = await Promise.all([
        api.get('/api/sensitive-words/policy'),
        api.get('/api/sensitive-words/rules'),
        api.get('/api/sensitive-words/groups'),
      ])
      const nextConfig = configRes.data?.data as Partial<SensitiveWordPolicy>
      const hasConfig =
        typeof nextConfig?.enabled === 'boolean' &&
        typeof nextConfig?.check_prompt === 'boolean'
      if (hasConfig) {
        const mergedConfig: SensitiveWordPolicy = {
          ...defaultConfigRef.current,
          ...nextConfig,
          block_message:
            nextConfig.block_message?.trim() || DEFAULT_BLOCK_MESSAGE,
        }
        setConfig(mergedConfig)
        initialConfigRef.current = mergedConfig
      }
      setRules((rulesRes.data?.data as RuleSummary[] | undefined) ?? [])
      setGroups((groupsRes.data?.data as string[] | undefined) ?? [])
    } catch (error) {
      toast.error(getErrorMessage(error, '无法加载敏感词策略'))
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  const parsedWords = useMemo(
    () => parseWords(draft.wordsText),
    [draft.wordsText]
  )

  const wordMatches = useMemo(
    () => findSensitiveWordMatches(draft.wordsText, wordSearch),
    [draft.wordsText, wordSearch]
  )
  const wordMatchesRef = useRef<SensitiveWordTextMatch[]>(wordMatches)
  wordMatchesRef.current = wordMatches

  const resetWordSearch = useCallback(() => {
    setWordSearch('')
    setActiveMatchIndex(0)
  }, [])

  const scrollTextareaMatchIntoView = useCallback(
    (textarea: HTMLTextAreaElement, start: number) => {
      const computedStyle = window.getComputedStyle(textarea)
      const parsedLineHeight = Number.parseFloat(computedStyle.lineHeight)
      const fontSize = Number.parseFloat(computedStyle.fontSize)
      let lineHeight = 20
      if (Number.isFinite(parsedLineHeight)) {
        lineHeight = parsedLineHeight
      } else if (Number.isFinite(fontSize)) {
        lineHeight = fontSize * 1.5
      }

      const lineNumber = textarea.value.slice(0, start).split('\n').length - 1
      const lineTop = lineNumber * lineHeight
      const lineBottom = lineTop + lineHeight
      const visibleTop = textarea.scrollTop
      const visibleBottom = visibleTop + textarea.clientHeight

      if (lineTop < visibleTop) {
        textarea.scrollTop = lineTop
      } else if (lineBottom > visibleBottom) {
        textarea.scrollTop = Math.max(
          0,
          lineTop - Math.max(0, (textarea.clientHeight - lineHeight) / 2)
        )
      }
    },
    []
  )

  const locateWordMatch = useCallback(
    (index: number) => {
      const match = wordMatchesRef.current[index]
      const textarea = textareaRef.current
      if (!match || !textarea) return

      const searchInput = searchInputRef.current
      const shouldRestoreSearchFocus = document.activeElement === searchInput

      if (shouldRestoreSearchFocus) {
        textarea.focus({ preventScroll: true })
      }
      textarea.setSelectionRange(match.start, match.end)
      scrollTextareaMatchIntoView(textarea, match.start)

      if (shouldRestoreSearchFocus) {
        searchInput?.focus({ preventScroll: true })
      }
    },
    [scrollTextareaMatchIntoView]
  )

  useEffect(() => {
    setActiveMatchIndex((current) => {
      if (wordMatches.length === 0) return 0
      return Math.min(current, wordMatches.length - 1)
    })
  }, [wordMatches.length])

  useEffect(() => {
    const currentMatches = wordMatchesRef.current
    if (!ruleDialogOpen || !wordSearch.trim() || currentMatches.length === 0) {
      return
    }
    if (document.activeElement === textareaRef.current) return
    locateWordMatch(Math.min(activeMatchIndex, currentMatches.length - 1))
  }, [activeMatchIndex, locateWordMatch, ruleDialogOpen, wordSearch])

  const saveConfig = async () => {
    const banThreshold = Math.max(
      1,
      Math.min(1000, Number(config.ban_threshold) || 50)
    )
    const retentionDays = Math.max(
      1,
      Math.min(3650, Number(config.full_prompt_retention_days) || 180)
    )
    const normalizedConfig: SensitiveWordPolicy = {
      ...config,
      ban_threshold: banThreshold,
      full_prompt_retention_days: retentionDays,
      max_prompt_runes: Math.max(
        1,
        Math.min(65536, Number(config.max_prompt_runes) || 65536)
      ),
      block_message: config.block_message.trim() || DEFAULT_BLOCK_MESSAGE,
    }
    const initialConfig = initialConfigRef.current ?? config
    const configChanged =
      JSON.stringify(normalizedConfig) !== JSON.stringify(initialConfig)
    if (!configChanged) return

    setIsSavingConfig(true)
    try {
      if (configChanged) {
        const response = await api.put('/api/sensitive-words/policy', {
          ...normalizedConfig,
        })
        const saved = response.data?.data as SensitiveWordPolicy | undefined
        if (saved) {
          setConfig(saved)
          initialConfigRef.current = saved
        } else {
          setConfig(normalizedConfig)
          initialConfigRef.current = normalizedConfig
        }
      }
      toast.success('敏感词策略已保存')
    } catch (error) {
      toast.error(getErrorMessage(error, '保存敏感词策略失败'))
    } finally {
      setIsSavingConfig(false)
    }
  }

  const openCreateDialog = () => {
    resetWordSearch()
    setDraft(emptyDraft())
    setRuleDialogOpen(true)
  }

  const openEditDialog = async (rule: RuleSummary) => {
    resetWordSearch()
    try {
      const response = await api.get(`/api/sensitive-words/rules/${rule.id}`)
      const detail = response.data?.data as RuleDetail | undefined
      if (!detail) throw new Error('规则详情为空')
      setDraft({
        id: detail.id,
        name: detail.name,
        wordsText: detail.words.join('\n'),
        scope: detail.scope,
        groups: detail.groups,
        mode: detail.mode,
      })
      resetWordSearch()
      setRuleDialogOpen(true)
    } catch (error) {
      toast.error(getErrorMessage(error, '无法加载规则详情'))
    }
  }

  const saveRule = async () => {
    const name = draft.name.trim()
    if (!name) {
      toast.error('请填写规则名称')
      return
    }
    if (parsedWords.words.length === 0) {
      toast.error('请至少填写一个有效敏感词')
      return
    }
    if (draft.scope === 'group' && draft.groups.length === 0) {
      toast.error('局部规则至少选择一个定价分组')
      return
    }

    setIsSavingRule(true)
    try {
      const payload = {
        name,
        words: parsedWords.words,
        scope: draft.scope,
        groups: draft.scope === 'group' ? draft.groups : [],
        mode: draft.mode,
      }
      if (draft.id) {
        await api.put(`/api/sensitive-words/rules/${draft.id}`, payload)
      } else {
        await api.post('/api/sensitive-words/rules', payload)
      }
      setRuleDialogOpen(false)
      resetWordSearch()
      await reload()
      toast.success(draft.id ? '敏感词规则已更新' : '敏感词规则已添加')
    } catch (error) {
      toast.error(getErrorMessage(error, '保存敏感词规则失败'))
    } finally {
      setIsSavingRule(false)
    }
  }

  const changeRuleMode = async (
    rule: RuleSummary,
    mode: RuleSummary['mode']
  ) => {
    setChangingModeRuleID(rule.id)
    try {
      await api.patch(`/api/sensitive-words/rules/${rule.id}/mode`, { mode })
      await reload()
      toast.success('规则处理模式已更新')
    } catch (error) {
      toast.error(getErrorMessage(error, '更新规则状态失败'))
    } finally {
      setChangingModeRuleID(null)
    }
  }

  const deleteRule = async () => {
    if (!deleteTarget) return
    setIsDeleting(true)
    try {
      await api.delete(`/api/sensitive-words/rules/${deleteTarget.id}`)
      setDeleteTarget(null)
      await reload()
      toast.success('敏感词规则已永久删除')
    } catch (error) {
      toast.error(getErrorMessage(error, '删除敏感词规则失败'))
    } finally {
      setIsDeleting(false)
    }
  }

  const updateSelectedGroups = (group: string, checked: boolean) => {
    setDraft((current) => {
      const nextGroups = checked
        ? [...new Set([...current.groups, group])]
        : current.groups.filter((item) => item !== group)
      return { ...current, groups: nextGroups }
    })
  }

  const handleWordFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return
    try {
      const text = await file.text()
      setDraft((current) => ({
        ...current,
        wordsText: current.wordsText
          ? `${current.wordsText.trimEnd()}\n${text}`
          : text,
      }))
      toast.success('TXT 词条已导入到编辑区')
    } catch {
      toast.error('读取 TXT 文件失败')
    } finally {
      event.target.value = ''
    }
  }

  const handleWordSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter' || event.nativeEvent.isComposing) return
    event.preventDefault()
    if (wordMatches.length === 0) return

    const backwards = event.shiftKey
    setActiveMatchIndex((current) =>
      getNextSensitiveWordMatchIndex(current, wordMatches.length, backwards)
    )
  }

  const currentMatchIndex =
    wordMatches.length === 0
      ? 0
      : Math.min(activeMatchIndex, wordMatches.length - 1)
  const matchCountLabel = `${wordMatches.length === 0 ? 0 : currentMatchIndex + 1}/${wordMatches.length}`
  const matchCountAriaLabel = t(
    'Sensitive word matches: {{current}} of {{total}}',
    {
      current: wordMatches.length === 0 ? 0 : currentMatchIndex + 1,
      total: wordMatches.length,
    }
  )

  return (
    <SettingsSection title='敏感词策略'>
      <SettingsPageFormActions
        onSave={() => void saveConfig()}
        isSaving={isSavingConfig}
        saveLabel='保存敏感词策略'
        savingLabel='正在保存'
      />

      <div className='space-y-6'>
        <section className='border-y py-4'>
          <div className='mb-4 flex flex-wrap items-center justify-between gap-2'>
            <div>
              <h3 className='text-sm font-semibold'>策略设置</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                规则版本 {config.version || 1}
                ；达到封禁阈值会禁用账号，但不会处理余额。
              </p>
            </div>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => {
                window.location.assign('/usage-logs/common?type=8')
              }}
            >
              查看关键词拦截日志
            </Button>
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            <div className='space-y-4'>
              <div className='flex items-center justify-between gap-4 border-b pb-3'>
                <span>
                  <span className='block text-sm font-medium'>
                    启用敏感词策略
                  </span>
                  <span className='text-muted-foreground block pt-1 text-xs'>
                    关闭后不进行匹配、拦截或记录。
                  </span>
                </span>
                <Switch
                  aria-label='Enable filtering'
                  checked={config.enabled}
                  onCheckedChange={(enabled) =>
                    setConfig((current) => ({ ...current, enabled }))
                  }
                />
              </div>
              <div className='flex items-center justify-between gap-4'>
                <span>
                  <span className='block text-sm font-medium'>检查提示词</span>
                  <span className='text-muted-foreground block pt-1 text-xs'>
                    只检查规范化后的文本请求内容。
                  </span>
                </span>
                <Switch
                  aria-label='Inspect user prompts'
                  disabled={!config.enabled}
                  checked={config.check_prompt}
                  onCheckedChange={(check_prompt) =>
                    setConfig((current) => ({ ...current, check_prompt }))
                  }
                />
              </div>
              <label className='flex items-center justify-between gap-4 border-t pt-3'>
                <span>
                  <span className='block text-sm font-medium'>
                    保存完整审计证据
                  </span>
                  <span className='text-muted-foreground block pt-1 text-xs'>
                    关闭后仍记录处理结果，但不保存提示词摘要和完整提示词。
                  </span>
                </span>
                <Switch
                  checked={config.retain_full_prompt}
                  onCheckedChange={(retain_full_prompt) =>
                    setConfig((current) => ({ ...current, retain_full_prompt }))
                  }
                />
              </label>
            </div>

            <div className='space-y-4'>
              <div className='grid gap-3 sm:grid-cols-3'>
                <div className='space-y-2'>
                  <Label htmlFor='sensitive-ban-threshold'>封禁阈值</Label>
                  <Input
                    id='sensitive-ban-threshold'
                    type='number'
                    min={1}
                    max={1000}
                    value={config.ban_threshold}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        ban_threshold: Number(event.target.value) || 1,
                      }))
                    }
                  />
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='sensitive-retention-days'>证据保留天数</Label>
                  <Input
                    id='sensitive-retention-days'
                    type='number'
                    min={1}
                    max={3650}
                    value={config.full_prompt_retention_days}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        full_prompt_retention_days:
                          Number(event.target.value) || 1,
                      }))
                    }
                  />
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='sensitive-max-prompt-runes'>
                    最大提示词长度
                  </Label>
                  <Input
                    id='sensitive-max-prompt-runes'
                    type='number'
                    min={1}
                    max={65536}
                    value={config.max_prompt_runes}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        max_prompt_runes: Number(event.target.value) || 1,
                      }))
                    }
                  />
                </div>
              </div>
            </div>
          </div>

          <div className='mt-4 space-y-2'>
            <Label htmlFor='sensitive-block-message'>客户端拦截提示</Label>
            <Textarea
              id='sensitive-block-message'
              value={config.block_message}
              rows={4}
              onChange={(event) =>
                setConfig((current) => ({
                  ...current,
                  block_message: event.target.value,
                }))
              }
            />
          </div>
        </section>

        <section className='space-y-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div>
              <h3 className='text-sm font-semibold'>敏感词规则</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                全局与局部规则统一管理。局部规则只能绑定分组定价中存在的分组。
              </p>
            </div>
            <Button type='button' size='sm' onClick={openCreateDialog}>
              <Plus data-icon='inline-start' />
              添加敏感词
            </Button>
          </div>

          <div className='overflow-x-auto rounded-md border'>
            <table className='w-full min-w-[780px] text-left text-sm'>
              <thead className='bg-muted/40 text-muted-foreground text-xs'>
                <tr>
                  <th className='px-3 py-2 font-medium'>规则名称</th>
                  <th className='px-3 py-2 font-medium'>范围</th>
                  <th className='px-3 py-2 font-medium'>使用分组</th>
                  <th className='px-3 py-2 font-medium'>词条数</th>
                  <th className='px-3 py-2 font-medium'>处理模式</th>
                  <th className='px-3 py-2 font-medium'>更新时间</th>
                  <th className='w-28 px-3 py-2 text-right font-medium'>
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className='divide-y'>
                {isLoading && (
                  <tr>
                    <td
                      colSpan={7}
                      className='text-muted-foreground px-3 py-10 text-center text-sm'
                    >
                      <Loader2 className='mr-2 inline size-4 animate-spin' />
                      正在加载规则
                    </td>
                  </tr>
                )}
                {!isLoading && rules.length === 0 && (
                  <tr>
                    <td
                      colSpan={7}
                      className='text-muted-foreground px-3 py-10 text-center text-sm'
                    >
                      尚未创建敏感词规则
                    </td>
                  </tr>
                )}
                {!isLoading &&
                  rules.length > 0 &&
                  rules.map((rule) => (
                    <tr key={rule.id} className='hover:bg-muted/30'>
                      <td className='max-w-64 px-3 py-2 font-medium'>
                        <span className='block truncate'>{rule.name}</span>
                      </td>
                      <td className='px-3 py-2'>
                        <Badge
                          variant={
                            rule.scope === 'global' ? 'secondary' : 'outline'
                          }
                        >
                          {rule.scope === 'global' ? '全局' : '指定分组'}
                        </Badge>
                      </td>
                      <td className='max-w-80 px-3 py-2'>
                        {rule.scope === 'global' ? (
                          <span className='text-muted-foreground'>
                            全部定价分组
                          </span>
                        ) : (
                          <div className='flex flex-wrap gap-1'>
                            {rule.groups.map((group) => (
                              <Badge key={group} variant='outline'>
                                {group}
                              </Badge>
                            ))}
                          </div>
                        )}
                      </td>
                      <td className='px-3 py-2 tabular-nums'>
                        {rule.word_count}
                      </td>
                      <td className='px-3 py-2'>
                        <Select
                          value={rule.mode}
                          disabled={changingModeRuleID === rule.id}
                          onValueChange={(value) => {
                            if (
                              value === 'block' ||
                              value === 'observe' ||
                              value === 'off'
                            ) {
                              void changeRuleMode(rule, value)
                            }
                          }}
                        >
                          <SelectTrigger className='w-24'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='block'>拦截</SelectItem>
                            <SelectItem value='observe'>观察</SelectItem>
                            <SelectItem value='off'>关闭</SelectItem>
                          </SelectContent>
                        </Select>
                      </td>
                      <td className='text-muted-foreground px-3 py-2 text-xs'>
                        {formatTime(rule.updated_at)}
                      </td>
                      <td className='px-3 py-2'>
                        <div className='flex justify-end gap-1'>
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <Button
                                  type='button'
                                  size='icon-sm'
                                  variant='ghost'
                                  aria-label='编辑规则'
                                  onClick={() => void openEditDialog(rule)}
                                />
                              }
                            >
                              <Pencil />
                            </TooltipTrigger>
                            <TooltipContent>编辑规则</TooltipContent>
                          </Tooltip>
                          <DropdownMenu>
                            <DropdownMenuTrigger
                              render={
                                <Button
                                  type='button'
                                  size='icon-sm'
                                  variant='ghost'
                                  aria-label='更多规则操作'
                                />
                              }
                            >
                              <MoreHorizontal />
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align='end'>
                              <DropdownMenuItem
                                onClick={() => void openEditDialog(rule)}
                              >
                                <Pencil />
                                编辑
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                              <DropdownMenuItem
                                variant='destructive'
                                onClick={() => setDeleteTarget(rule)}
                              >
                                <Trash2 />
                                永久删除
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </section>
      </div>

      <Dialog
        open={ruleDialogOpen}
        onOpenChange={(open) => {
          setRuleDialogOpen(open)
          if (!open) resetWordSearch()
        }}
        title={draft.id ? '编辑敏感词规则' : '添加敏感词规则'}
        description='一个规则可包含多个词条，并可设为全局或绑定多个定价分组。'
        contentClassName='sm:max-w-3xl'
        contentHeight='min(70dvh, 680px)'
        bodyClassName='space-y-5'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => {
                setRuleDialogOpen(false)
                resetWordSearch()
              }}
              disabled={isSavingRule}
            >
              取消
            </Button>
            <Button
              type='button'
              onClick={() => void saveRule()}
              disabled={isSavingRule}
            >
              {isSavingRule && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              保存规则
            </Button>
          </>
        }
      >
        <div className='grid gap-4 sm:grid-cols-[minmax(0,1fr)_13rem]'>
          <div className='space-y-2'>
            <Label htmlFor='sensitive-rule-name'>规则名称</Label>
            <Input
              id='sensitive-rule-name'
              maxLength={64}
              value={draft.name}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  name: event.target.value,
                }))
              }
              placeholder='例如：通用违规词库'
            />
          </div>
          <div className='space-y-2'>
            <Label>规则范围</Label>
            <Select
              value={draft.scope}
              onValueChange={(scope) =>
                setDraft((current) => ({
                  ...current,
                  scope: scope === 'group' ? 'group' : 'global',
                  groups: scope === 'group' ? current.groups : [],
                }))
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='global'>全局</SelectItem>
                <SelectItem value='group'>指定分组</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {draft.scope === 'group' && (
          <div className='space-y-2'>
            <Label>使用分组</Label>
            <Popover>
              <PopoverTrigger
                render={
                  <Button
                    type='button'
                    variant='outline'
                    className='w-full justify-between'
                  />
                }
              >
                <span className='truncate'>
                  {draft.groups.length === 0
                    ? '选择定价分组'
                    : `已选择 ${draft.groups.length} 个分组`}
                </span>
              </PopoverTrigger>
              <PopoverContent
                align='start'
                className='max-h-64 w-[var(--anchor-width)] overflow-y-auto'
              >
                {groups.length === 0 ? (
                  <p className='text-muted-foreground px-1 py-2 text-xs'>
                    当前没有可用定价分组
                  </p>
                ) : (
                  groups.map((group) => (
                    <label
                      key={group}
                      className='hover:bg-muted flex cursor-pointer items-center gap-2 rounded-md px-1 py-1.5'
                    >
                      <Checkbox
                        checked={draft.groups.includes(group)}
                        onCheckedChange={(checked) =>
                          updateSelectedGroups(group, checked === true)
                        }
                      />
                      <span className='min-w-0 truncate text-sm'>{group}</span>
                    </label>
                  ))
                )}
              </PopoverContent>
            </Popover>
            {draft.groups.length > 0 && (
              <div className='flex flex-wrap gap-1'>
                {draft.groups.map((group) => (
                  <Badge key={group} variant='outline'>
                    {group}
                  </Badge>
                ))}
              </div>
            )}
          </div>
        )}

        <div className='space-y-2'>
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <Label htmlFor='sensitive-rule-words'>敏感词条</Label>
            <div className='flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto'>
              <div className='relative w-full sm:w-60'>
                <Search
                  aria-hidden='true'
                  className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
                />
                <Input
                  ref={searchInputRef}
                  type='text'
                  value={wordSearch}
                  onChange={(event) => {
                    setWordSearch(event.target.value)
                    setActiveMatchIndex(0)
                  }}
                  onKeyDown={handleWordSearchKeyDown}
                  placeholder={t('Search current words...')}
                  aria-label={t('Search current words...')}
                  className='pr-20 pl-8'
                />
                <span
                  role='status'
                  aria-live='polite'
                  aria-label={matchCountAriaLabel}
                  className='text-muted-foreground pointer-events-none absolute top-1/2 right-8 -translate-y-1/2 text-xs tabular-nums'
                >
                  {matchCountLabel}
                </span>
                {wordSearch && (
                  <Button
                    type='button'
                    size='icon-sm'
                    variant='ghost'
                    className='absolute top-1/2 right-0.5 size-7 -translate-y-1/2'
                    aria-label={t('Clear search')}
                    title={t('Clear search')}
                    onClick={() => {
                      resetWordSearch()
                      searchInputRef.current?.focus()
                    }}
                  >
                    <X aria-hidden='true' />
                  </Button>
                )}
              </div>
              <input
                ref={fileInputRef}
                type='file'
                accept='.txt,text/plain'
                className='hidden'
                onChange={(event) => void handleWordFile(event)}
              />
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={() => fileInputRef.current?.click()}
              >
                <FileUp data-icon='inline-start' />
                导入 TXT
              </Button>
            </div>
          </div>
          <Textarea
            id='sensitive-rule-words'
            ref={textareaRef}
            rows={12}
            value={draft.wordsText}
            onChange={(event) =>
              setDraft((current) => ({
                ...current,
                wordsText: event.target.value,
              }))
            }
            placeholder={'每行一个敏感词\n支持直接粘贴或导入 TXT'}
          />
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
            <span>有效词条 {parsedWords.words.length}</span>
            <span>重复 {parsedWords.duplicateCount}</span>
            <span>空行 {parsedWords.blankCount}</span>
            <span>超长 {parsedWords.tooLongCount}</span>
          </div>
        </div>

        <div className='grid gap-3 border-t pt-4 sm:grid-cols-2'>
          <div className='space-y-2'>
            <Label>处理模式</Label>
            <Select
              value={draft.mode}
              onValueChange={(mode) => {
                if (mode === 'block' || mode === 'observe' || mode === 'off') {
                  setDraft((current) => ({ ...current, mode }))
                }
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='block'>拦截</SelectItem>
                <SelectItem value='observe'>观察</SelectItem>
                <SelectItem value='off'>关闭</SelectItem>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              关闭仅停用当前规则；全局审计证据由策略区统一控制。
            </p>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !isDeleting) setDeleteTarget(null)
        }}
        title='永久删除敏感词规则'
        desc={
          deleteTarget
            ? `“${deleteTarget.name}”及其 ${deleteTarget.word_count} 个词条和分组绑定将被删除，历史关键词拦截日志会保留。`
            : ''
        }
        confirmText='永久删除'
        destructive
        isLoading={isDeleting}
        handleConfirm={() => void deleteRule()}
      />
    </SettingsSection>
  )
}

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
  Power,
  PowerOff,
  Trash2,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
} from 'react'
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

const DEFAULT_BLOCK_MESSAGE =
  '你的请求因命中敏感词已被拦截，已记录 1 次；累计超过 5 次将立即封号，余额不退，如果有攻击破解别人网站等情节严重的情况将会直接报警。请勿使用当前分组进行违规对话；如有误判，请联系群主审核并清理你的记录。'

type SensitiveWordConfig = {
  enabled: boolean
  check_prompt: boolean
  mode: 'block' | 'observe' | 'off'
  audit_enabled: boolean
  block_message: string
  ban_threshold: number
  full_prompt_retention_days: number
  max_prompt_runes: number
  rule_version: number
}

type RuleSummary = {
  id: number
  name: string
  scope: 'global' | 'group'
  groups: string[]
  word_count: number
  enabled: boolean
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
  enabled: boolean
}

type ParsedWords = {
  words: string[]
  blankCount: number
  duplicateCount: number
  tooLongCount: number
}

type Props = {
  defaultValues: {
    CheckSensitiveEnabled: boolean
    CheckSensitiveOnPromptEnabled: boolean
    SensitiveWords?: string
  }
}

const emptyDraft = (): RuleDraft => ({
  name: '',
  wordsText: '',
  scope: 'global',
  groups: [],
  enabled: true,
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

function RulePowerIcon(props: { enabled: boolean; isLoading: boolean }) {
  if (props.isLoading) return <Loader2 className='animate-spin' />
  if (props.enabled) return <PowerOff />
  return <Power />
}

export function SensitiveWordsSection({ defaultValues }: Props) {
  const [config, setConfig] = useState<SensitiveWordConfig>({
    enabled: defaultValues.CheckSensitiveEnabled,
    check_prompt: defaultValues.CheckSensitiveOnPromptEnabled,
    mode: 'block',
    audit_enabled: true,
    block_message: DEFAULT_BLOCK_MESSAGE,
    ban_threshold: 5,
    full_prompt_retention_days: 180,
    max_prompt_runes: 65536,
    rule_version: 1,
  })
  const [rules, setRules] = useState<RuleSummary[]>([])
  const [groups, setGroups] = useState<string[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSavingConfig, setIsSavingConfig] = useState(false)
  const [isSavingRule, setIsSavingRule] = useState(false)
  const [togglingRuleID, setTogglingRuleID] = useState<number | null>(null)
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false)
  const [draft, setDraft] = useState<RuleDraft>(emptyDraft)
  const [deleteTarget, setDeleteTarget] = useState<RuleSummary | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const reload = useCallback(async () => {
    setIsLoading(true)
    try {
      const [configRes, rulesRes, groupsRes] = await Promise.all([
        api.get('/api/sensitive-words/config'),
        api.get('/api/sensitive-words/rules'),
        api.get('/api/sensitive-words/groups'),
      ])
      const nextConfig = configRes.data?.data as Partial<SensitiveWordConfig>
      setConfig((current) => ({
        ...current,
        ...nextConfig,
        mode:
          nextConfig?.mode === 'observe' || nextConfig?.mode === 'off'
            ? nextConfig.mode
            : 'block',
        block_message:
          nextConfig?.block_message?.trim() || DEFAULT_BLOCK_MESSAGE,
      }))
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

  const saveConfig = async () => {
    const banThreshold = Math.max(
      1,
      Math.min(1000, Number(config.ban_threshold) || 5)
    )
    const retentionDays = Math.max(
      1,
      Math.min(3650, Number(config.full_prompt_retention_days) || 180)
    )
    setIsSavingConfig(true)
    try {
      const response = await api.put('/api/sensitive-words/config', {
        ...config,
        ban_threshold: banThreshold,
        full_prompt_retention_days: retentionDays,
        max_prompt_runes: 65536,
        block_message: config.block_message.trim() || DEFAULT_BLOCK_MESSAGE,
      })
      const saved = response.data?.data as SensitiveWordConfig | undefined
      if (saved) setConfig(saved)
      toast.success('敏感词策略已保存')
    } catch (error) {
      toast.error(getErrorMessage(error, '保存敏感词策略失败'))
    } finally {
      setIsSavingConfig(false)
    }
  }

  const openCreateDialog = () => {
    setDraft(emptyDraft())
    setRuleDialogOpen(true)
  }

  const openEditDialog = async (rule: RuleSummary) => {
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
        enabled: detail.enabled,
      })
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
        enabled: draft.enabled,
      }
      if (draft.id) {
        await api.put(`/api/sensitive-words/rules/${draft.id}`, payload)
      } else {
        await api.post('/api/sensitive-words/rules', payload)
      }
      setRuleDialogOpen(false)
      await reload()
      toast.success(draft.id ? '敏感词规则已更新' : '敏感词规则已添加')
    } catch (error) {
      toast.error(getErrorMessage(error, '保存敏感词规则失败'))
    } finally {
      setIsSavingRule(false)
    }
  }

  const toggleRule = async (rule: RuleSummary) => {
    setTogglingRuleID(rule.id)
    try {
      await api.patch(`/api/sensitive-words/rules/${rule.id}/status`, {
        enabled: !rule.enabled,
      })
      await reload()
      toast.success(rule.enabled ? '规则已停用' : '规则已启用')
    } catch (error) {
      toast.error(getErrorMessage(error, '更新规则状态失败'))
    } finally {
      setTogglingRuleID(null)
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

  return (
    <SettingsSection title='敏感词策略'>
      <SettingsPageFormActions
        onSave={() => void saveConfig()}
        isSaving={isSavingConfig}
        saveLabel='保存策略'
        savingLabel='正在保存'
      />

      <div className='space-y-6'>
        <section className='border-y py-4'>
          <div className='mb-4 flex flex-wrap items-center justify-between gap-2'>
            <div>
              <h3 className='text-sm font-semibold'>策略设置</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                规则版本 {config.rule_version || 1}
                ；第五次有效拦截会禁用账号，但不会处理余额。
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
              <label className='flex items-center justify-between gap-4 border-b pb-3'>
                <span>
                  <span className='block text-sm font-medium'>
                    启用敏感词策略
                  </span>
                  <span className='text-muted-foreground block pt-1 text-xs'>
                    关闭后不进行匹配、拦截或记录。
                  </span>
                </span>
                <Switch
                  checked={config.enabled}
                  onCheckedChange={(enabled) =>
                    setConfig((current) => ({ ...current, enabled }))
                  }
                />
              </label>
              <label className='flex items-center justify-between gap-4'>
                <span>
                  <span className='block text-sm font-medium'>检查提示词</span>
                  <span className='text-muted-foreground block pt-1 text-xs'>
                    只检查规范化后的文本请求内容。
                  </span>
                </span>
                <Switch
                  checked={config.check_prompt}
                  onCheckedChange={(check_prompt) =>
                    setConfig((current) => ({ ...current, check_prompt }))
                  }
                />
              </label>
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
                  checked={config.audit_enabled}
                  onCheckedChange={(audit_enabled) =>
                    setConfig((current) => ({ ...current, audit_enabled }))
                  }
                />
              </label>
            </div>

            <div className='space-y-4'>
              <div className='space-y-2'>
                <Label>处理模式</Label>
                <div
                  className='inline-flex rounded-md border p-1'
                  role='group'
                  aria-label='处理模式'
                >
                  {(
                    [
                      ['block', '拦截'],
                      ['observe', '观察'],
                      ['off', '关闭'],
                    ] as const
                  ).map(([value, label]) => (
                    <Button
                      key={value}
                      type='button'
                      size='sm'
                      variant={config.mode === value ? 'secondary' : 'ghost'}
                      onClick={() =>
                        setConfig((current) => ({ ...current, mode: value }))
                      }
                    >
                      {label}
                    </Button>
                  ))}
                </div>
              </div>
              <div className='grid grid-cols-2 gap-3'>
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
                  <th className='px-3 py-2 font-medium'>状态</th>
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
                        <Badge variant={rule.enabled ? 'secondary' : 'outline'}>
                          {rule.enabled ? '启用' : '停用'}
                        </Badge>
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
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <Button
                                  type='button'
                                  size='icon-sm'
                                  variant='ghost'
                                  aria-label={
                                    rule.enabled ? '停用规则' : '启用规则'
                                  }
                                  disabled={togglingRuleID === rule.id}
                                  onClick={() => void toggleRule(rule)}
                                />
                              }
                            >
                              <RulePowerIcon
                                enabled={rule.enabled}
                                isLoading={togglingRuleID === rule.id}
                              />
                            </TooltipTrigger>
                            <TooltipContent>
                              {rule.enabled ? '停用规则' : '启用规则'}
                            </TooltipContent>
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
                              <DropdownMenuItem
                                onClick={() => void toggleRule(rule)}
                              >
                                {rule.enabled ? <PowerOff /> : <Power />}
                                {rule.enabled ? '停用' : '启用'}
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
        onOpenChange={setRuleDialogOpen}
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
              onClick={() => setRuleDialogOpen(false)}
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
          <div className='flex items-center justify-between gap-2'>
            <Label htmlFor='sensitive-rule-words'>敏感词条</Label>
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
          <Textarea
            id='sensitive-rule-words'
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

        <label className='flex items-center justify-between gap-4 border-t pt-4'>
          <span>
            <span className='block text-sm font-medium'>创建后立即启用</span>
            <span className='text-muted-foreground block pt-1 text-xs'>
              停用的规则仍保留，且不会参与请求匹配。
            </span>
          </span>
          <Switch
            checked={draft.enabled}
            onCheckedChange={(enabled) =>
              setDraft((current) => ({ ...current, enabled }))
            }
          />
        </label>
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

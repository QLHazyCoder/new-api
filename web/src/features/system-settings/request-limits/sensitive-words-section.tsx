import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SettingsSection } from '../components/settings-section'
import { Dialog } from '@/components/dialog'

type Rule = { id: number; word: string; scope: string; enabled: boolean; groups?: { group_name: string }[] }
type Audit = { id: number; user_id: number; created_at: string; username_snapshot: string; group_name: string; model_name: string; matched_words: string; matched_scope: string; whitelist_bypassed: boolean; blocked: boolean; violation_count: number; redacted_preview: string }
type Stats = { total_rules: number; global_rules: number; group_rules: number; whitelist_users: number; today_hits: number; today_blocks: number; today_whitelist: number; today_auto_bans: number }
type Props = { defaultValues: { CheckSensitiveEnabled: boolean; CheckSensitiveOnPromptEnabled: boolean; SensitiveWords?: string } }

export function SensitiveWordsSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const [enabled, setEnabled] = useState(defaultValues.CheckSensitiveEnabled)
  const [auditEnabled, setAuditEnabled] = useState(true)
  const [rules, setRules] = useState<Rule[]>([])
  const [groups, setGroups] = useState<string[]>([])
  const [audits, setAudits] = useState<Audit[]>([])
  const [whitelist, setWhitelist] = useState<{ id: number; user_id: number; enabled: boolean; remark: string }[]>([])
  const [word, setWord] = useState('')
  const [scope, setScope] = useState('global')
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  const [userId, setUserId] = useState('')
  const [message, setMessage] = useState('')
  const [stats, setStats] = useState<Stats>({ total_rules: 0, global_rules: 0, group_rules: 0, whitelist_users: 0, today_hits: 0, today_blocks: 0, today_whitelist: 0, today_auto_bans: 0 })
  const [detail, setDetail] = useState<any>(null)

  async function reload() {
    const [ruleRes, groupRes, whitelistRes, auditRes, configRes, statsRes] = await Promise.all([
      api.get('/api/sensitive-words/rules'), api.get('/api/sensitive-words/groups'), api.get('/api/sensitive-words/whitelist'),
      api.get('/api/sensitive-words/audits', { params: { page: 1, page_size: 20 } }), api.get('/api/sensitive-words/config'), api.get('/api/sensitive-words/stats'),
    ])
    setRules(ruleRes.data?.data ?? []); setGroups(groupRes.data?.data ?? []); setWhitelist(whitelistRes.data?.data ?? []); setAudits(auditRes.data?.data?.items ?? [])
    setEnabled(configRes.data?.data?.enabled ?? enabled); setAuditEnabled(configRes.data?.data?.audit_enabled ?? true); setMessage(configRes.data?.data?.block_message ?? '')
    setStats(statsRes.data?.data ?? stats)
  }
  useEffect(() => { void reload() }, [])

  async function saveConfig() {
    await api.put('/api/sensitive-words/config', { enabled, audit_enabled: auditEnabled, block_message: message || undefined, ban_threshold: 5, full_prompt_retention_days: 180, max_prompt_runes: 65536 })
    await reload()
  }
  async function addRule() {
    if (!word.trim() || (scope === 'group' && selectedGroups.length === 0)) return
    const words = [...new Set(word.split(/\r?\n/).map((item) => item.trim()).filter(Boolean))]
    for (const item of words) await api.post('/api/sensitive-words/rules', { word: item, scope, groups: selectedGroups })
    setWord(''); setSelectedGroups([]); await reload()
  }
  async function addWhitelist() {
    const id = Number(userId); if (!id) return
    await api.post('/api/sensitive-words/whitelist', { user_id: id, enabled: true, remark: '管理员白名单' }); setUserId(''); await reload()
  }
  async function viewAudit(id: number) {
    const res = await api.get(`/api/sensitive-words/audits/${id}`)
    const event = res.data?.data
    if (event) setDetail(event)
  }

  return <SettingsSection title={t('Sensitive Words')}>
    <div className='space-y-6'>
      <div className='grid gap-3 md:grid-cols-4'>
        <div className='rounded-md border p-3'><div className='text-xs text-muted-foreground'>规则总数</div><div className='text-2xl font-semibold'>{stats.total_rules}</div></div>
        <div className='rounded-md border p-3'><div className='text-xs text-muted-foreground'>今日命中 / 拦截</div><div className='text-2xl font-semibold'>{stats.today_hits} / {stats.today_blocks}</div></div>
        <div className='rounded-md border p-3'><div className='text-xs text-muted-foreground'>白名单放行</div><div className='text-2xl font-semibold'>{stats.today_whitelist}</div></div>
        <div className='rounded-md border p-3'><div className='text-xs text-muted-foreground'>今日自动封禁</div><div className='text-2xl font-semibold'>{stats.today_auto_bans}</div></div>
      </div>
      <div className='flex flex-wrap items-center gap-5 rounded-md border p-4'>
        <label className='flex items-center gap-2 text-sm'><Switch checked={enabled} onCheckedChange={setEnabled} />启用敏感词拦截</label>
        <label className='flex items-center gap-2 text-sm'><Switch checked={auditEnabled} onCheckedChange={setAuditEnabled} />保存审计证据</label>
        <Button onClick={() => void saveConfig()}>保存配置</Button>
      </div>
      <div className='space-y-3 rounded-md border p-4'>
        <div className='flex items-center justify-between'><h3 className='font-semibold'>添加敏感词</h3><span className='text-xs text-muted-foreground'>第五次非白名单命中自动封号并清零余额</span></div>
        <div className='grid gap-3 md:grid-cols-[1fr_180px_1fr_auto]'>
          <Textarea value={word} onChange={(e) => setWord(e.target.value)} placeholder='每行一个敏感词，可批量粘贴' rows={2} />
          <Select value={scope} onValueChange={(value) => setScope(value ?? 'global')}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value='global'>全局规则</SelectItem><SelectItem value='group'>指定分组</SelectItem></SelectContent></Select>
          {scope === 'group' ? <select multiple value={selectedGroups} onChange={(e) => setSelectedGroups(Array.from(e.target.selectedOptions).map((o) => o.value))} className='h-10 rounded-md border bg-background px-3 text-sm'>{groups.map((group) => <option key={group} value={group}>{group}</option>)}</select> : <div />}
          <Button onClick={() => void addRule()}>添加</Button>
        </div>
        <Textarea value={message} onChange={(e) => setMessage(e.target.value)} rows={2} placeholder='客户端拦截提示' />
      </div>
      <div className='rounded-md border'><div className='border-b p-4 font-semibold'>敏感词规则</div><div className='divide-y'>{rules.map((rule) => <div key={rule.id} className='flex flex-wrap items-center justify-between gap-3 p-3 text-sm'><span className='font-medium'>{rule.word}</span><span>{rule.scope === 'global' ? '全局' : `局部：${rule.groups?.map((g) => g.group_name).join(', ')}`}</span><span>{rule.enabled ? '启用' : '停用'}</span><Button variant='ghost' size='sm' onClick={() => void api.delete(`/api/sensitive-words/rules/${rule.id}`).then(reload)}>删除</Button></div>)}</div></div>
      <div className='space-y-3 rounded-md border p-4'><div className='flex items-center gap-3'><h3 className='font-semibold'>用户白名单</h3><Input className='max-w-40' value={userId} onChange={(e) => setUserId(e.target.value)} placeholder='用户 ID' /><Button onClick={() => void addWhitelist()}>加入白名单</Button></div><p className='text-xs text-muted-foreground'>白名单用户仍保存审计记录，但不拦截、不增加违规次数。</p><div className='divide-y'>{whitelist.map((item) => <div key={item.id} className='flex items-center justify-between py-2 text-sm'><span>用户 ID：{item.user_id}</span><span>{item.enabled ? '启用' : '停用'}</span><Button variant='ghost' size='sm' onClick={() => void api.delete(`/api/sensitive-words/whitelist/${item.user_id}`).then(reload)}>移除</Button></div>)}</div></div>
      <div className='rounded-md border'><div className='border-b p-4 font-semibold'>最近审计记录</div><div className='divide-y'>{audits.map((event) => <button key={event.id} className='flex w-full items-center justify-between gap-3 p-3 text-left text-sm hover:bg-muted/40' onClick={() => void viewAudit(event.id)}><span>{event.username_snapshot || event.user_id} · {event.group_name || '—'}</span><span>{event.matched_words || '—'}</span><span>{event.whitelist_bypassed ? '白名单放行' : event.blocked ? `已拦截，第 ${event.violation_count} 次` : '放行'}</span><span className='max-w-xs truncate text-muted-foreground'>{event.redacted_preview}</span></button>)}</div></div>
      <Dialog open={Boolean(detail)} onOpenChange={(open) => { if (!open) setDetail(null) }} title='审计详情' contentClassName='max-w-4xl' contentHeight='80vh' bodyClassName='space-y-4 overflow-y-auto'>
        {detail && <><div className='grid gap-2 text-sm md:grid-cols-2'><div>请求 ID：{detail.request_id || '—'}</div><div>用户：{detail.username_snapshot || detail.user_id}</div><div>分组：{detail.group_name || '—'}</div><div>模型：{detail.model_name || '—'}</div><div>关键词：{detail.matched_words || '—'}</div><div>处理结果：{detail.whitelist_bypassed ? '白名单放行' : detail.blocked ? `已拦截，第 ${detail.violation_count} 次` : '放行'}</div></div><div><div className='mb-1 text-sm font-medium'>脱敏摘要</div><pre className='max-h-32 overflow-auto rounded-md bg-muted p-3 text-xs whitespace-pre-wrap'>{detail.redacted_preview || '无'}</pre></div><div><div className='mb-1 text-sm font-medium'>规范化完整提示词</div><pre className='max-h-96 overflow-auto rounded-md border p-3 text-xs whitespace-pre-wrap'>{detail.full_prompt || '无'}</pre></div></>}
      </Dialog>
    </div>
  </SettingsSection>
}

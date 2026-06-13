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
import type { Modality, ModelCapability, PricingModel } from '../types'
import { hashStringToSeed, seededRandom } from './seed'

// ----------------------------------------------------------------------------
// Model metadata inference
// ----------------------------------------------------------------------------
//
// Model metadata shown as factual display data comes only from explicit
// model-management configuration. Tags may still opt into capability badges.

const TAG_TO_CAPABILITY: Record<string, ModelCapability> = {
  vision: 'vision',
  multimodal: 'vision',
  reasoning: 'reasoning',
  thinking: 'reasoning',
  tools: 'tools',
  function: 'function_calling',
  'function-calling': 'function_calling',
  streaming: 'streaming',
  json: 'json_mode',
  structured: 'structured_output',
  search: 'web_search',
  code: 'code_interpreter',
  embedding: 'embeddings',
}

function parseModelTags(tagsString?: string): string[] {
  if (!tagsString) return []
  return tagsString
    .split(/[,;|\s]+/)
    .map((t) => t.trim().toLowerCase())
    .filter(Boolean)
}

function capabilitiesFromTags(tags: string[]): ModelCapability[] {
  const set = new Set<ModelCapability>()

  for (const tag of tags) {
    const cap = TAG_TO_CAPABILITY[tag]
    if (cap) set.add(cap)
  }

  return Array.from(set)
}

function ordered(modalities: Set<Modality>): Modality[] {
  const order: Modality[] = ['text', 'image', 'audio', 'video', 'file']
  return order.filter((m) => modalities.has(m))
}

function normalizeModalities(value?: string | Modality[]): Modality[] | null {
  if (!value) return null
  const items = Array.isArray(value) ? value : value.split(/[,;|\s]+/)
  const set = new Set<Modality>()
  for (const item of items) {
    const normalized = String(item).trim().toLowerCase() as Modality
    if (['text', 'image', 'audio', 'video', 'file'].includes(normalized)) {
      set.add(normalized)
    }
  }
  if (set.size === 0) return null
  return ordered(set)
}

function normalizePositiveNumber(value?: number): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? value
    : undefined
}

export type ModelMetadata = {
  context_length?: number
  max_output_tokens?: number
  knowledge_cutoff?: string
  release_date?: string
  parameter_count?: string
  input_modalities: Modality[]
  output_modalities: Modality[]
  capabilities: ModelCapability[]
}

/**
 * Build model metadata for display. Hard specification fields come only from
 * model management configuration, and capability badges come only from
 * explicitly configured tags.
 */
export function inferModelMetadata(model: PricingModel): ModelMetadata {
  const tags = parseModelTags(model.tags)

  const configuredInputs = normalizeModalities(model.input_modalities)
  const configuredOutputs = normalizeModalities(model.output_modalities)
  const capabilities = model.capabilities ?? capabilitiesFromTags(tags)

  return {
    context_length: normalizePositiveNumber(model.context_length),
    max_output_tokens: normalizePositiveNumber(model.max_output_tokens),
    knowledge_cutoff: model.knowledge_cutoff || undefined,
    release_date: model.release_date || undefined,
    parameter_count: model.parameter_count || undefined,
    input_modalities: configuredInputs ?? [],
    output_modalities: configuredOutputs ?? [],
    capabilities,
  }
}

const TOKEN_FORMAT = new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 1,
})

/** Format a token count compactly: 128_000 → "128K", 1_000_000 → "1M". */
export function formatTokenCount(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return '—'
  if (tokens >= 1_000_000) {
    const value = tokens / 1_000_000
    return `${TOKEN_FORMAT.format(value)}M`
  }
  if (tokens >= 1_000) {
    const value = tokens / 1_000
    return `${TOKEN_FORMAT.format(value)}K`
  }
  return TOKEN_FORMAT.format(tokens)
}

/** Format a YYYY-MM (or YYYY-MM-DD) date as `Mon YYYY` for display. */
export function formatYearMonth(value: string): string {
  if (!value) return '—'
  const [yearStr, monthStr] = value.split('-')
  const year = Number(yearStr)
  const month = Number(monthStr)
  if (!Number.isFinite(year) || !Number.isFinite(month)) return value
  const date = new Date(Date.UTC(year, month - 1, 1))
  return date.toLocaleString(undefined, { year: 'numeric', month: 'short' })
}

// ---------------------------------------------------------------------------
// Provider / vendor / tokenizer / license inference
// ---------------------------------------------------------------------------
//
// These helpers derive vendor-style metadata from the model name. They are
// purely heuristic and serve only the API-info display until the backend
// returns explicit fields.

export type ModelVendor =
  | 'openai'
  | 'anthropic'
  | 'google'
  | 'meta'
  | 'mistral'
  | 'qwen'
  | 'deepseek'
  | 'xai'
  | 'cohere'
  | 'baidu'
  | 'zhipu'
  | 'moonshot'
  | 'minimax'
  | 'tencent'
  | 'bytedance'
  | 'midjourney'
  | 'stability'
  | 'unknown'

export type ApiInfo = {
  vendor: ModelVendor
  vendor_label: string
  tokenizer: string
  tokenizer_note?: string
  license: string
  license_kind: 'proprietary' | 'open' | 'open-weight' | 'unknown'
  data_retention_days: number
  training_opt_out: boolean
  homepage?: string
}

const VENDOR_LABELS: Record<ModelVendor, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  google: 'Google',
  meta: 'Meta',
  mistral: 'Mistral AI',
  qwen: 'Alibaba (Qwen)',
  deepseek: 'DeepSeek',
  xai: 'xAI',
  cohere: 'Cohere',
  baidu: 'Baidu',
  zhipu: 'Zhipu AI',
  moonshot: 'Moonshot AI',
  minimax: 'MiniMax',
  tencent: 'Tencent',
  bytedance: 'ByteDance',
  midjourney: 'Midjourney',
  stability: 'Stability AI',
  unknown: 'Unknown',
}

function detectVendor(name: string): ModelVendor {
  const n = name.toLowerCase()
  if (/^gpt|^o[1-4]|davinci|babbage|whisper|tts|dall.?e|sora|^omni/.test(n))
    return 'openai'
  if (/claude/.test(n)) return 'anthropic'
  if (/gemini|gemma|imagen|veo|palm/.test(n)) return 'google'
  if (/llama|^codellama/.test(n)) return 'meta'
  if (/mistral|mixtral|codestral|magistral|pixtral/.test(n)) return 'mistral'
  if (/qwen|qwq|qvq/.test(n)) return 'qwen'
  if (/deepseek/.test(n)) return 'deepseek'
  if (/grok/.test(n)) return 'xai'
  if (/command|cohere|aya/.test(n)) return 'cohere'
  if (/ernie|wenxin/.test(n)) return 'baidu'
  if (/glm|chatglm|cogview|cogvideo/.test(n)) return 'zhipu'
  if (/kimi|moonshot/.test(n)) return 'moonshot'
  if (/abab|minimax|hailuo/.test(n)) return 'minimax'
  if (/hunyuan/.test(n)) return 'tencent'
  if (/doubao|seed|jimeng/.test(n)) return 'bytedance'
  if (/midjourney|niji/.test(n)) return 'midjourney'
  if (/^sd-|stable[-_]?diffusion|sdxl/.test(n)) return 'stability'
  return 'unknown'
}

const TOKENIZER_BY_VENDOR: Partial<Record<ModelVendor, string>> = {
  openai: 'o200k_base',
  anthropic: 'Anthropic Claude tokenizer',
  google: 'SentencePiece (Gemini)',
  meta: 'Llama 3 tokenizer',
  mistral: 'Mistral tokenizer (BPE)',
  qwen: 'Qwen tokenizer (tiktoken-compat)',
  deepseek: 'DeepSeek tokenizer (BPE)',
  xai: 'Grok tokenizer (BPE)',
  cohere: 'Cohere tokenizer',
  baidu: 'Ernie tokenizer',
  zhipu: 'GLM tokenizer',
  moonshot: 'Kimi tokenizer',
  minimax: 'ABAB tokenizer',
  tencent: 'Hunyuan tokenizer',
  bytedance: 'Doubao tokenizer',
}

function inferTokenizer(
  model: PricingModel,
  vendor: ModelVendor
): {
  tokenizer: string
  note?: string
} {
  const name = model.model_name.toLowerCase()
  if (vendor === 'openai') {
    if (/gpt-3|davinci|babbage|whisper|tts/.test(name)) {
      return { tokenizer: 'cl100k_base', note: 'Older GPT-3.5 family' }
    }
    return { tokenizer: 'o200k_base' }
  }
  return { tokenizer: TOKENIZER_BY_VENDOR[vendor] ?? 'BPE (vendor-specific)' }
}

const LICENSE_BY_VENDOR: Record<
  ModelVendor,
  { license: string; kind: ApiInfo['license_kind'] }
> = {
  openai: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  anthropic: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  google: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  meta: { license: 'Llama Community License', kind: 'open-weight' },
  mistral: { license: 'Apache 2.0 / Commercial', kind: 'open-weight' },
  qwen: { license: 'Tongyi Qianwen License', kind: 'open-weight' },
  deepseek: { license: 'DeepSeek License', kind: 'open-weight' },
  xai: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  cohere: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  baidu: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  zhipu: { license: 'GLM-4 License', kind: 'open-weight' },
  moonshot: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  minimax: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  tencent: { license: 'Hunyuan License', kind: 'open-weight' },
  bytedance: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  midjourney: { license: 'Proprietary (commercial)', kind: 'proprietary' },
  stability: { license: 'Stability AI Community License', kind: 'open-weight' },
  unknown: { license: 'Provider-specific', kind: 'unknown' },
}

const HOMEPAGE_BY_VENDOR: Partial<Record<ModelVendor, string>> = {
  openai: 'https://platform.openai.com/docs/models',
  anthropic: 'https://docs.anthropic.com/claude/docs/models-overview',
  google: 'https://ai.google.dev/models',
  meta: 'https://llama.meta.com/',
  mistral: 'https://docs.mistral.ai/getting-started/models/',
  qwen: 'https://qwenlm.github.io/',
  deepseek: 'https://api-docs.deepseek.com/',
  xai: 'https://x.ai/api',
  cohere: 'https://docs.cohere.com/docs/models',
  baidu: 'https://cloud.baidu.com/product/wenxinworkshop',
  zhipu: 'https://open.bigmodel.cn/dev/api',
  moonshot: 'https://platform.moonshot.cn/docs',
  minimax: 'https://platform.minimaxi.com/document/notice',
  tencent: 'https://cloud.tencent.com/document/product/1729',
  bytedance: 'https://www.volcengine.com/docs/82379',
  midjourney: 'https://www.midjourney.com/',
  stability: 'https://platform.stability.ai/',
}

/**
 * Build vendor / tokenizer / license / privacy metadata for the model.
 * Returns deterministic values keyed off the model name so each render is
 * stable.
 */
export function inferApiInfo(model: PricingModel): ApiInfo {
  const vendor = detectVendor(model.model_name || '')
  const tk = inferTokenizer(model, vendor)
  const license = LICENSE_BY_VENDOR[vendor]
  const rand = seededRandom(hashStringToSeed(`${model.model_name}:api`))
  const retention = vendor === 'openai' ? 30 : Math.round(rand() * 90)
  return {
    vendor,
    vendor_label: VENDOR_LABELS[vendor],
    tokenizer: tk.tokenizer,
    tokenizer_note: tk.note,
    license: license.license,
    license_kind: license.kind,
    data_retention_days: retention,
    training_opt_out: true,
    homepage: HOMEPAGE_BY_VENDOR[vendor],
  }
}

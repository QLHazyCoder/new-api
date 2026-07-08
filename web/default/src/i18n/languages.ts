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

export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zh-CN', label: '简体中文' },
  { code: 'en', label: 'English' },
  { code: 'fr', label: 'Français' },
  { code: 'ru', label: 'Русский' },
  { code: 'ja', label: '日本語' },
  { code: 'vi', label: 'Tiếng Việt' },
  { code: 'zh-TW', label: '繁體中文' },
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

function normalizeChineseLocale(value: string): 'zh-CN' | 'zh-TW' | undefined {
  const normalized = value.trim().replaceAll('_', '-').toLowerCase()
  if (
    normalized === 'zh-tw' ||
    normalized === 'zh-hk' ||
    normalized === 'zh-mo' ||
    normalized === 'zhtw' ||
    normalized.startsWith('zh-hant')
  ) {
    return 'zh-TW'
  }
  if (
    normalized === 'zh' ||
    normalized === 'zh-cn' ||
    normalized === 'zh-hans' ||
    normalized === 'zhcn' ||
    normalized.startsWith('zh-hans')
  ) {
    return 'zh-CN'
  }
  return undefined
}

export function normalizeInterfaceLanguage(value?: string | null): string {
  if (!value) return 'en'

  const chineseLocale = normalizeChineseLocale(value)
  if (chineseLocale) return chineseLocale

  const normalized = value.trim().replaceAll('_', '-').toLowerCase()
  const exact = INTERFACE_LANGUAGE_OPTIONS.find(
    (lang) => lang.code.toLowerCase() === normalized
  )
  if (exact) return exact.code

  const baseLanguage = normalized.split('-')[0]
  const base = INTERFACE_LANGUAGE_OPTIONS.find(
    (lang) => lang.code.toLowerCase() === baseLanguage
  )
  return base?.code ?? 'en'
}

export function convertDetectedLanguage(value: string): string {
  return normalizeInterfaceLanguage(value)
}

export function toIntlLocale(
  value?: Intl.LocalesArgument | null
): Intl.LocalesArgument | undefined {
  if (!value) return undefined

  if (Array.isArray(value)) {
    const locales = value
      .map((locale) => toIntlLocale(locale))
      .filter((locale): locale is string | Intl.Locale => Boolean(locale))
    return locales.length > 0 ? locales : undefined
  }

  if (typeof value !== 'string') {
    return value
  }

  const locale = value.trim().replaceAll('_', '-')
  const normalized = normalizeChineseLocale(locale) ?? locale

  try {
    return Intl.getCanonicalLocales(normalized)[0] ?? undefined
  } catch {
    return undefined
  }
}

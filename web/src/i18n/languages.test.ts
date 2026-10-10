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
import { describe, expect, test } from 'vitest'

import {
  convertDetectedLanguage,
  INTERFACE_LANGUAGE_OPTIONS,
  toIntlLocale,
} from './languages'

describe('convertDetectedLanguage', () => {
  test('maps browser BCP-47 Chinese tags onto interface codes', () => {
    expect(convertDetectedLanguage('zh-TW')).toBe('zh-TW')
    expect(convertDetectedLanguage('zh-HK')).toBe('zh-TW')
    expect(convertDetectedLanguage('zh-MO')).toBe('zh-TW')
    expect(convertDetectedLanguage('zh-Hant-TW')).toBe('zh-TW')
    expect(convertDetectedLanguage('zh')).toBe('zh-CN')
    expect(convertDetectedLanguage('zh-CN')).toBe('zh-CN')
    expect(convertDetectedLanguage('zh-Hans')).toBe('zh-CN')
    expect(convertDetectedLanguage('zh_Hant')).toBe('zh-TW')
  })

  test('keeps already-normalized interface codes stable (localStorage round-trip)', () => {
    for (const { code } of INTERFACE_LANGUAGE_OPTIONS) {
      expect(convertDetectedLanguage(code)).toBe(code)
    }
    expect(convertDetectedLanguage('zhTW')).toBe('zh-TW')
    expect(convertDetectedLanguage('zhCN')).toBe('zh-CN')
  })

  test('normalizes regional languages to the supported interface codes', () => {
    expect(convertDetectedLanguage('en')).toBe('en')
    expect(convertDetectedLanguage('fr-FR')).toBe('fr')
    expect(convertDetectedLanguage('ja')).toBe('ja')
    expect(convertDetectedLanguage('unknown')).toBe('en')
  })

  test('keeps legacy Chinese values safe for Intl formatters', () => {
    expect(toIntlLocale('zhTW')).toBe('zh-TW')
    expect(toIntlLocale('zhCN')).toBe('zh-CN')
    expect(toIntlLocale(['zhTW', 'en-US'])).toEqual(['zh-TW', 'en-US'])
  })
})

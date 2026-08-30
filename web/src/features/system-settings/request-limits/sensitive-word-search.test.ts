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
*/
import { describe, expect, test } from 'vitest'

import {
  findSensitiveWordMatches,
  getNextSensitiveWordMatchIndex,
} from './sensitive-word-search'

describe('findSensitiveWordMatches', () => {
  test('returns no matches for an empty or whitespace-only query', () => {
    expect(findSensitiveWordMatches('alpha\nbeta', '')).toEqual([])
    expect(findSensitiveWordMatches('alpha\nbeta', '   ')).toEqual([])
  })

  test('matches mixed Chinese and English text without case sensitivity', () => {
    expect(findSensitiveWordMatches('安全Rule\n安全rule', 'RULE')).toEqual([
      { start: 2, end: 6 },
      { start: 9, end: 13 },
    ])
  })

  test('returns UTF-16 offsets across multiple lines', () => {
    const text = '第一行\nSensitive word\n最后一行'
    expect(findSensitiveWordMatches(text, 'SENSITIVE')).toEqual([
      { start: 4, end: 13 },
    ])
    expect(findSensitiveWordMatches('😀Sensitive', 'sensitive')).toEqual([
      { start: 2, end: 11 },
    ])
  })

  test('returns non-overlapping matches in document order', () => {
    expect(findSensitiveWordMatches('foo FOO foo', 'foo')).toEqual([
      { start: 0, end: 3 },
      { start: 4, end: 7 },
      { start: 8, end: 11 },
    ])
  })

  test('treats regex-looking characters as literal text', () => {
    expect(findSensitiveWordMatches('keep a.*b literal', 'a.*b')).toEqual([
      { start: 5, end: 9 },
    ])
  })

  test('does not alter text and reports no result when absent', () => {
    const text = '保留原始内容\nwithout a match'
    expect(findSensitiveWordMatches(text, 'missing')).toEqual([])
    expect(text).toBe('保留原始内容\nwithout a match')
  })

  test('keeps the original range when lowercase expands a character', () => {
    expect(findSensitiveWordMatches('İstanbul', 'i\u0307')).toEqual([
      { start: 0, end: 1 },
    ])
  })

  test('cycles forward and backward through match indexes', () => {
    expect(getNextSensitiveWordMatchIndex(0, 3)).toBe(1)
    expect(getNextSensitiveWordMatchIndex(2, 3)).toBe(0)
    expect(getNextSensitiveWordMatchIndex(0, 3, true)).toBe(2)
    expect(getNextSensitiveWordMatchIndex(2, 3, true)).toBe(1)
    expect(getNextSensitiveWordMatchIndex(0, 0)).toBe(0)
  })
})

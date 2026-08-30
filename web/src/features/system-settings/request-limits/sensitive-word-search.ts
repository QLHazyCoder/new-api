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

export type SensitiveWordTextMatch = {
  start: number
  end: number
}

type NormalizedText = {
  value: string
  starts: number[]
  ends: number[]
}

/**
 * Lowercase text while retaining the original UTF-16 range for every
 * normalized code unit. Textarea selections use UTF-16 offsets, so this keeps
 * selection positions correct for non-ASCII text as well as ordinary ASCII.
 */
function normalizeWithOffsets(text: string): NormalizedText {
  let value = ''
  const starts: number[] = []
  const ends: number[] = []

  for (let offset = 0; offset < text.length; ) {
    const codePoint = text.codePointAt(offset)
    if (codePoint === undefined) break

    const character = String.fromCodePoint(codePoint)
    const nextOffset = offset + character.length
    const normalizedCharacter = character.toLowerCase()
    value += normalizedCharacter

    for (let index = 0; index < normalizedCharacter.length; index += 1) {
      starts.push(offset)
      ends.push(nextOffset)
    }

    offset = nextOffset
  }

  return { value, starts, ends }
}

/**
 * Finds non-overlapping, case-insensitive plain-text matches. Whitespace-only
 * queries are treated as empty so an accidental space does not select the
 * entire draft.
 */
export function findSensitiveWordMatches(
  text: string,
  query: string
): SensitiveWordTextMatch[] {
  const normalizedQuery = query.trim().toLowerCase()
  if (!normalizedQuery) return []

  const normalizedText = normalizeWithOffsets(text)
  const matches: SensitiveWordTextMatch[] = []
  let searchFrom = 0

  while (searchFrom < normalizedText.value.length) {
    const matchStart = normalizedText.value.indexOf(normalizedQuery, searchFrom)
    if (matchStart < 0) break

    const matchEnd = matchStart + normalizedQuery.length
    const start = normalizedText.starts[matchStart]
    const end = normalizedText.ends[matchEnd - 1]
    if (start === undefined || end === undefined) break

    matches.push({ start, end })
    searchFrom = matchEnd
  }

  return matches
}

/** Returns the next match index, wrapping at either end of the result list. */
export function getNextSensitiveWordMatchIndex(
  currentIndex: number,
  matchCount: number,
  backwards = false
): number {
  if (matchCount <= 0) return 0

  const normalizedIndex =
    ((currentIndex % matchCount) + matchCount) % matchCount
  return backwards
    ? (normalizedIndex - 1 + matchCount) % matchCount
    : (normalizedIndex + 1) % matchCount
}

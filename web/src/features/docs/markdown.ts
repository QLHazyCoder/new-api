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
const IMAGE_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.avif']

export function isSecureDocsImageURL(
  value: string,
  requireImageExtension = false
): boolean {
  try {
    const url = new URL(value)
    if (
      url.protocol !== 'https:' ||
      !url.hostname ||
      url.username ||
      url.password
    ) {
      return false
    }
    if (!requireImageExtension) return true

    const pathname = url.pathname.toLowerCase()
    return IMAGE_EXTENSIONS.some((extension) => pathname.endsWith(extension))
  } catch {
    return false
  }
}

function standaloneImage(line: string): string | null {
  if (/^https:\/\/[^\s<>]+$/i.test(line) && isSecureDocsImageURL(line, true)) {
    return `![](${line})`
  }

  const linkedURL = line.match(
    /^\[(https:\/\/[^\s\]]+)\]\((https:\/\/[^\s)]+)\)$/i
  )
  if (
    linkedURL &&
    linkedURL[1] === linkedURL[2] &&
    isSecureDocsImageURL(linkedURL[2], true)
  ) {
    return `![](${linkedURL[2]})`
  }

  return null
}

export function normalizeStandaloneDocsImages(markdown: string): string {
  const lines = markdown.split('\n')
  let fence: { marker: string; length: number } | null = null

  return lines
    .map((line) => {
      const fenceLine = line.match(/^\s{0,3}(`{3,}|~{3,})/)
      if (fence) {
        if (
          fenceLine &&
          fenceLine[1][0] === fence.marker &&
          fenceLine[1].length >= fence.length
        ) {
          fence = null
        }
        return line
      }
      if (fenceLine) {
        fence = { marker: fenceLine[1][0], length: fenceLine[1].length }
        return line
      }
      return standaloneImage(line) ?? line
    })
    .join('\n')
}

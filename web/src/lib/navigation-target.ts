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
export interface NavigationTarget {
  href: string
  external: boolean
}

export function resolveNavigationTarget(
  href: string,
  origin: string
): NavigationTarget {
  try {
    const target = new URL(href, origin)
    const currentOrigin = new URL(origin).origin
    if (
      target.origin === currentOrigin &&
      ['http:', 'https:'].includes(target.protocol)
    ) {
      return {
        href: `${target.pathname}${target.search}${target.hash}`,
        external: false,
      }
    }
  } catch {
    // Preserve malformed administrator-configured URLs as external links.
  }

  return { href, external: true }
}

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

import { resolveNavigationTarget } from '../navigation-target'

describe('resolveNavigationTarget', () => {
  test('same-origin absolute documentation URLs become internal route paths', () => {
    expect(
      resolveNavigationTarget(
        'https://coderapi.vip/docs?source=nav#start',
        'https://coderapi.vip'
      )
    ).toEqual({ href: '/docs?source=nav#start', external: false })
  })

  test('root-relative links remain internal routes', () => {
    expect(resolveNavigationTarget('/docs', 'https://coderapi.vip')).toEqual({
      href: '/docs',
      external: false,
    })
  })

  test('foreign documentation URLs remain external links', () => {
    const href = 'https://docs.newapi.pro'
    expect(resolveNavigationTarget(href, 'https://coderapi.vip')).toEqual({
      href,
      external: true,
    })
  })
})

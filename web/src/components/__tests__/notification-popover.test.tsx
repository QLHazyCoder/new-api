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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { NotificationPopover } from '../notification-popover'

const animationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterEach(() => {
  if (animationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      animationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

function NotificationCenter(props: { empty?: boolean; loading?: boolean }) {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<'notice' | 'announcements'>('notice')
  return (
    <>
      <button type='button'>Outside action</button>
      <NotificationPopover
        open={open}
        onOpenChange={setOpen}
        unreadCount={props.empty ? 0 : 2}
        activeTab={tab}
        onTabChange={setTab}
        notice={props.empty ? '' : 'Current **service notice** with details.'}
        announcements={
          props.empty
            ? []
            : [
                {
                  id: 1,
                  content: 'Timeline announcement with **important details**.',
                  extra: 'Additional context',
                  publishDate: '2026-10-21T08:00:00Z',
                },
              ]
        }
        loading={props.loading === true}
        noticeUpdated={!props.empty}
        announcementsUpdated={!props.empty}
      />
    </>
  )
}

describe('notification center dialog', () => {
  test('the bell opens a centered modal with bounded width, scrollable content and reachable close action', async () => {
    const user = userEvent.setup()
    render(<NotificationCenter />)
    const trigger = screen.getByRole('button', { name: 'Notifications' })
    await user.click(trigger)
    const dialog = screen.getByRole('dialog', { name: 'System Announcements' })
    expect(dialog).toHaveAccessibleDescription(
      'Latest platform updates and notices'
    )
    expect(dialog).toHaveClass(
      'top-[var(--dialog-viewport-center,50dvh)]',
      'left-1/2',
      '-translate-x-1/2',
      '-translate-y-1/2',
      'max-w-[calc(100%-2rem)]',
      'sm:max-w-2xl',
      'overflow-y-auto'
    )
    const scrollArea = dialog.querySelector('[data-slot=scroll-area]')
    expect(scrollArea).toHaveClass('h-[min(60dvh,36rem)]')
    const footer = dialog.querySelector<HTMLElement>(
      '[data-slot=dialog-footer]'
    )
    expect(footer).toHaveClass('flex-shrink-0')
    expect(footer).toContainElement(
      within(dialog).getByRole('button', { name: 'Close' })
    )
    expect(scrollArea).not.toContainElement(footer)
    await user.click(within(dialog).getByRole('button', { name: 'Close' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await waitFor(() => expect(trigger).toHaveFocus())
  })

  test('both updated tabs remain marked and keyboard navigation selects the timeline', async () => {
    const user = userEvent.setup()
    render(<NotificationCenter />)
    await user.click(screen.getByRole('button', { name: 'Notifications' }))
    const dialog = screen.getByRole('dialog', { name: 'System Announcements' })
    const noticeTab = within(dialog).getByRole('tab', { name: /^Notice/ })
    const timelineTab = within(dialog).getByRole('tab', { name: /^Timeline/ })
    expect(noticeTab).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(noticeTab).toHaveFocus())
    await user.keyboard('{ArrowRight}')
    expect(timelineTab).toHaveFocus()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(timelineTab).toHaveAttribute('aria-selected', 'true')
    )
    expect(timelineTab).toHaveFocus()
    expect(within(dialog).getByText('Additional context')).toBeVisible()
    expect(within(dialog).getAllByText('New update')).toHaveLength(2)
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  test('outside presses do not dismiss the announcement before explicit close', async () => {
    const user = userEvent.setup()
    render(<NotificationCenter />)
    await user.click(screen.getByRole('button', { name: 'Notifications' }))
    const backdrop = document.querySelector('[data-slot=dialog-overlay]')
    expect(backdrop).not.toBeNull()
    await user.click(backdrop as HTMLElement)
    expect(
      screen.getByRole('dialog', { name: 'System Announcements' })
    ).toBeVisible()
  })

  test.each([
    { empty: true, loading: false, message: 'No announcements at this time' },
    { empty: false, loading: true, message: 'Loading...' },
  ])(
    'empty=$empty and loading=$loading keep the close action usable',
    async (scenario) => {
      const user = userEvent.setup()
      render(
        <NotificationCenter empty={scenario.empty} loading={scenario.loading} />
      )
      await user.click(screen.getByRole('button', { name: 'Notifications' }))
      const dialog = screen.getByRole('dialog', {
        name: 'System Announcements',
      })
      expect(within(dialog).getByText(scenario.message)).toBeVisible()
      await user.click(within(dialog).getByRole('button', { name: 'Close' }))
      await waitFor(() =>
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      )
    }
  )
})

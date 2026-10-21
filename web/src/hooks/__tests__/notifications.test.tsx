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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

import { useNotifications } from '../use-notifications'

type ApiMethod = (url: string) => Promise<{ data: unknown }>
type MockableApi = { get: ApiMethod }

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const queryClients: QueryClient[] = []

let noticeContent = ''
let announcementContent = ''
let failNotice = false
let failStatus = false
let requests: string[] = []

function wrapper(queryClient: QueryClient) {
  return function QueryWrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {props.children}
      </QueryClientProvider>
    )
  }
}

function createQueryClient(): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  queryClients.push(client)
  return client
}

beforeEach(() => {
  window.localStorage.clear()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'alice', role: 1 })
  noticeContent = ''
  announcementContent = ''
  failNotice = false
  failStatus = false
  requests = []
  apiClient.get = async (url) => {
    requests.push(url)
    if (url === '/api/notice') {
      if (failNotice) throw new Error('Notice unavailable')
      return { data: { success: true, data: noticeContent } }
    }
    if (url === '/api/status') {
      if (failStatus) throw new Error('Status unavailable')
      return {
        data: {
          success: true,
          data: {
            announcements_enabled: true,
            announcements: announcementContent
              ? [
                  {
                    id: 1,
                    content: announcementContent,
                    type: 'default',
                    publishDate: '2026-10-21T08:00:00Z',
                  },
                ]
              : [],
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  }
})

afterEach(() => {
  cleanup()
  queryClients.splice(0).forEach((client) => client.clear())
  apiClient.get = originalGet
  useAuthStore.getState().auth.reset()
  useNotificationStore.setState(useNotificationStore.getInitialState(), true)
  window.localStorage.clear()
})

describe('notification dialog confirmation', () => {
  test('only a notice update selects Notice and confirms only when closed', async () => {
    noticeContent = 'Current service notice'
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.activeTab).toBe('notice')
    expect(result.current.noticeUpdated).toBe(true)
    expect(result.current.announcementsUpdated).toBe(false)
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBeNull()

    act(() => result.current.setActiveTab('announcements'))
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBeNull()
    act(() => result.current.closePopover())
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBe(
      noticeContent
    )
    expect(result.current.unreadCount).toBe(0)
  })

  test('only a timeline update selects Timeline, including editing an existing ID', async () => {
    announcementContent = 'Original announcement'
    const first = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(first.result.current.popoverOpen).toBe(true))
    expect(first.result.current.activeTab).toBe('announcements')
    act(() => first.result.current.closePopover())
    first.unmount()

    announcementContent = 'Edited announcement with unchanged date and ID'
    const second = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(second.result.current.popoverOpen).toBe(true))
    expect(second.result.current.activeTab).toBe('announcements')
    expect(second.result.current.announcements[0]?.content).toBe(
      announcementContent
    )
  })

  test('both updates share one dialog and keep both tab update indicators until close', async () => {
    noticeContent = 'Current notice'
    announcementContent = 'Current announcement'
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.noticeUpdated).toBe(true)
    expect(result.current.announcementsUpdated).toBe(true)
    expect(result.current.unreadCount).toBe(2)
    act(() => result.current.setActiveTab('announcements'))
    expect(result.current.unreadCount).toBe(2)
    act(() => result.current.closePopover())
    expect(result.current.unreadCount).toBe(0)
  })

  test('the confirmed current version does not reopen on a later visit', async () => {
    noticeContent = 'Current notice'
    announcementContent = 'Current announcement'
    const first = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(first.result.current.popoverOpen).toBe(true))
    act(() => first.result.current.closePopover())
    first.unmount()

    const nextVisit = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(nextVisit.result.current.loading).toBe(false))
    expect(nextVisit.result.current.popoverOpen).toBe(false)
    expect(nextVisit.result.current.unreadCount).toBe(0)
  })

  test('disabled notification headers do not fetch notification sources', async () => {
    const { result } = renderHook(() => useNotifications(false), {
      wrapper: wrapper(createQueryClient()),
    })
    expect(result.current.popoverOpen).toBe(false)
    expect(result.current.loading).toBe(true)
    expect(requests).toEqual([])
  })

  test('signed out visitors can open the dialog manually but never automatically', async () => {
    useAuthStore.getState().auth.reset()
    noticeContent = 'Public notice'
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.popoverOpen).toBe(false)
    act(() => result.current.openPopover())
    expect(result.current.popoverOpen).toBe(true)
    expect(result.current.notice).toBe('Public notice')
    act(() => result.current.closePopover())
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBe(
      'Public notice'
    )
  })

  test('failed refreshes do not automatically open using cached content', async () => {
    failNotice = true
    failStatus = true
    const client = createQueryClient()
    client.setQueryData(['notice'], {
      success: true,
      data: 'Unconfirmed cached notice',
    })
    client.setQueryData(['status'], {
      announcements_enabled: true,
      announcements: [{ id: 1, content: 'Unconfirmed cached announcement' }],
    })
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    await waitFor(() =>
      expect(client.getQueryState(['status'])?.status).toBe('error')
    )
    expect(result.current.popoverOpen).toBe(false)
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBeNull()
  })

  test('returning to the page refreshes even fresh cached queries and selects only the changed tab', async () => {
    noticeContent = 'Confirmed notice'
    announcementContent = 'Confirmed announcement'
    const client = createQueryClient()
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    act(() => result.current.setActiveTab('announcements'))
    act(() => result.current.closePopover())

    noticeContent = 'Changed notice'
    act(() => window.dispatchEvent(new Event('focus')))
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.activeTab).toBe('notice')
    expect(result.current.announcementsUpdated).toBe(false)
    act(() => result.current.closePopover())

    announcementContent = 'Changed timeline content'
    const visibility = vi.spyOn(document, 'visibilityState', 'get')
    visibility.mockReturnValue('hidden')
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    expect(result.current.popoverOpen).toBe(false)
    visibility.mockReturnValue('visible')
    act(() => document.dispatchEvent(new Event('visibilitychange')))
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.activeTab).toBe('announcements')
    expect(result.current.noticeUpdated).toBe(false)
  })

  test('updates arriving while the dialog is open are frozen and remain unconfirmed until the next visit', async () => {
    noticeContent = 'Notice version one'
    const client = createQueryClient()
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    noticeContent = 'Notice version two'
    act(() => window.dispatchEvent(new Event('focus')))
    await waitFor(() =>
      expect(client.getQueryData(['notice'])).toMatchObject({
        data: 'Notice version two',
      })
    )
    expect(result.current.notice).toBe('Notice version one')
    act(() => result.current.closePopover())
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBe(
      'Notice version one'
    )
    expect(result.current.popoverOpen).toBe(false)

    act(() => window.dispatchEvent(new Event('focus')))
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.notice).toBe('Notice version two')
  })

  test('entering waits for both refreshes instead of opening stale cached content', async () => {
    const client = createQueryClient()
    client.setQueryData(['notice'], {
      success: true,
      data: 'Cached notice',
    })
    const endpoint = apiClient.get
    let resolveNotice!: (value: { data: unknown }) => void
    const response = new Promise<{ data: unknown }>((resolve) => {
      resolveNotice = resolve
    })
    apiClient.get = (url) => {
      if (url === '/api/notice') return response
      return endpoint(url)
    }
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    await waitFor(() =>
      expect(client.isFetching({ queryKey: ['notice'] })).toBe(1)
    )
    expect(result.current.popoverOpen).toBe(false)
    await act(async () => {
      resolveNotice({ data: { success: true, data: 'Fresh notice' } })
      await response
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.notice).toBe('Fresh notice')
  })

  test('a failed source is not confirmed when the other successfully updated source opens the dialog', async () => {
    failNotice = true
    announcementContent = 'Timeline update'
    const client = createQueryClient()
    client.setQueryData(['notice'], { success: true, data: 'Stale notice' })
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.activeTab).toBe('announcements')
    expect(result.current.noticeUpdated).toBe(false)
    act(() => result.current.closePopover())
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBeNull()
    expect(
      useNotificationStore.getState().confirmedAnnouncementsVersion
    ).not.toBeNull()
  })

  test('sign-out while checking prevents auto opening and does not confirm the pending response', async () => {
    let resolveResponse!: (value: { data: unknown }) => void
    const response = new Promise<{ data: unknown }>((resolve) => {
      resolveResponse = resolve
    })
    apiClient.get = () => response
    const client = createQueryClient()
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(client),
    })
    act(() => useAuthStore.getState().auth.reset())
    await act(async () => {
      resolveResponse({ data: { success: true, data: 'Late notice' } })
      await response
    })
    await waitFor(() => expect(client.isFetching()).toBe(0))
    expect(result.current.popoverOpen).toBe(false)
    expect(useNotificationStore.getState().confirmedNoticeVersion).toBeNull()
  })

  test('entering shares the existing status request and has no duplicate initial fetch', async () => {
    noticeContent = 'New notice'
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(requests.filter((url) => url === '/api/notice')).toHaveLength(1)
    expect(requests.filter((url) => url === '/api/status')).toHaveLength(1)
  })

  test('local confirmation survives hydration and unchanged content stays closed', async () => {
    noticeContent = 'Persisted notice'
    announcementContent = 'Persisted announcement'
    const first = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(first.result.current.popoverOpen).toBe(true))
    act(() => first.result.current.closePopover())
    first.unmount()
    const persisted = window.localStorage.getItem('notification-storage')
    useNotificationStore.setState(useNotificationStore.getInitialState(), true)
    window.localStorage.setItem('notification-storage', persisted as string)
    await useNotificationStore.persist.rehydrate()
    const nextVisit = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(nextVisit.result.current.loading).toBe(false))
    expect(nextVisit.result.current.popoverOpen).toBe(false)
    expect(nextVisit.result.current.unreadCount).toBe(0)
  })

  test('legacy notice confirmation is preserved but old timeline IDs require a content confirmation once', async () => {
    noticeContent = 'Already read notice'
    announcementContent = 'Existing timeline content'
    window.localStorage.setItem(
      'notification-storage',
      JSON.stringify({
        state: {
          lastReadNotice: noticeContent,
          readAnnouncementKeys: ['id:1'],
          closedUntilDate: 'Mon Oct 19 2026',
        },
        version: 0,
      })
    )
    await useNotificationStore.persist.rehydrate()
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.popoverOpen).toBe(true))
    expect(result.current.activeTab).toBe('announcements')
    expect(result.current.noticeUpdated).toBe(false)
    expect(result.current.announcementsUpdated).toBe(true)
    act(() => result.current.closePopover())
    expect(result.current.unreadCount).toBe(0)
  })

  test('a disabled timeline does not trigger or confirm hidden announcements', async () => {
    const endpoint = apiClient.get
    apiClient.get = (url) => {
      if (url === '/api/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              announcements_enabled: false,
              announcements: [{ id: 1, content: 'Disabled announcement' }],
            },
          },
        })
      }
      return endpoint(url)
    }
    const { result } = renderHook(() => useNotifications(), {
      wrapper: wrapper(createQueryClient()),
    })
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.popoverOpen).toBe(false)
    expect(result.current.announcements).toEqual([])
    expect(
      useNotificationStore.getState().confirmedAnnouncementsVersion
    ).toBeNull()
  })
})

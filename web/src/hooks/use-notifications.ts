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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { getNotice } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'
import { statusQueryOptions } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useNotificationStore } from '@/stores/notification-store'

interface AnnouncementItem {
  id?: number | string
  type?: string
  content?: string
  extra?: string
  publishDate?: string | Date
}

interface NotificationSnapshot {
  notice: string
  noticeVersion: string | null
  announcements: AnnouncementItem[]
  announcementsVersion: string | null
}

function getAnnouncementsVersion(announcements: AnnouncementItem[]): string {
  return JSON.stringify(
    announcements.map((item) => ({
      id: item.id ?? null,
      content: (item.content || '').trim(),
      extra: (item.extra || '').trim(),
      type: item.type || '',
      publishDate: item.publishDate || '',
    }))
  )
}

function hasUnconfirmedNotice(
  snapshot: NotificationSnapshot,
  confirmed: string | null
) {
  return (
    snapshot.noticeVersion !== null &&
    snapshot.noticeVersion !== confirmed &&
    (snapshot.notice.length > 0 || confirmed !== null)
  )
}

function hasUnconfirmedAnnouncements(
  snapshot: NotificationSnapshot,
  confirmed: string | null
) {
  return (
    snapshot.announcementsVersion !== null &&
    snapshot.announcementsVersion !== confirmed &&
    (snapshot.announcements.length > 0 || confirmed !== null)
  )
}

/**
 * Reads the current Notice and timeline versions. A dialog snapshot is frozen
 * when opened and only those versions are confirmed when the user closes it.
 */
export function useNotifications(enabled = true) {
  const queryClient = useQueryClient()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const isAuthenticated = userId !== null

  const noticeQuery = useQuery({
    queryKey: ['notice'],
    queryFn: async () => requireServerSuccess(await getNotice()),
    staleTime: 1000 * 60 * 5,
    enabled,
  })
  const statusQuery = useQuery({ ...statusQueryOptions, enabled })

  const [popoverOpen, setPopoverOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'notice' | 'announcements'>(
    'notice'
  )
  const [latestSnapshot, setLatestSnapshot] =
    useState<NotificationSnapshot | null>(null)
  const [dialogSnapshot, setDialogSnapshotState] =
    useState<NotificationSnapshot | null>(null)
  const isDialogOpenRef = useRef(false)
  const isMountedRef = useRef(false)
  const checkGenerationRef = useRef(0)
  const checkPromiseRef = useRef<Promise<void> | null>(null)
  const latestSnapshotRef = useRef<NotificationSnapshot | null>(null)
  const dialogSnapshotRef = useRef<NotificationSnapshot | null>(null)
  const activeTabRef = useRef(activeTab)

  const confirmedNoticeVersion = useNotificationStore(
    (state) => state.confirmedNoticeVersion
  )
  const confirmedAnnouncementsVersion = useNotificationStore(
    (state) => state.confirmedAnnouncementsVersion
  )
  const setLatest = useCallback((snapshot: NotificationSnapshot) => {
    latestSnapshotRef.current = snapshot
    setLatestSnapshot(snapshot)
  }, [])

  const setDialogSnapshot = useCallback(
    (snapshot: NotificationSnapshot | null) => {
      dialogSnapshotRef.current = snapshot
      setDialogSnapshotState(snapshot)
    },
    []
  )

  const setTab = useCallback((tab: 'notice' | 'announcements') => {
    activeTabRef.current = tab
    setActiveTab(tab)
  }, [])

  const announcements = useMemo<AnnouncementItem[]>(() => {
    const status = statusQuery.data
    if (!status || status.announcements_enabled !== true) return []
    return ((status.announcements || []) as AnnouncementItem[]).slice(0, 20)
  }, [statusQuery.data])

  const currentQuerySnapshot = useMemo<NotificationSnapshot>(() => {
    const notice =
      noticeQuery.data?.success === true
        ? (noticeQuery.data.data || '').trim()
        : ''
    const timelineEnabled = statusQuery.data?.announcements_enabled === true
    return {
      notice,
      noticeVersion: noticeQuery.isSuccess ? notice : null,
      announcements,
      announcementsVersion:
        statusQuery.isSuccess && timelineEnabled
          ? getAnnouncementsVersion(announcements)
          : null,
    }
  }, [
    announcements,
    noticeQuery.data,
    noticeQuery.isSuccess,
    statusQuery.data,
    statusQuery.isSuccess,
  ])

  const checkForUpdates = useCallback((): Promise<void> => {
    if (!enabled || !isAuthenticated) return Promise.resolve()
    if (checkPromiseRef.current) return checkPromiseRef.current
    const generation = checkGenerationRef.current

    const checkPromise = (async () => {
      const [noticeResult, statusResult] = await Promise.allSettled([
        queryClient.fetchQuery({
          queryKey: ['notice'],
          queryFn: async () => requireServerSuccess(await getNotice()),
          staleTime: 0,
        }),
        queryClient.fetchQuery({ ...statusQueryOptions, staleTime: 0 }),
      ])

      // A pending public request may finish after navigation or sign-out.
      if (
        !isMountedRef.current ||
        generation !== checkGenerationRef.current ||
        useAuthStore.getState().auth.user?.id !== userId
      ) {
        return
      }

      const noticeSucceeded = noticeResult.status === 'fulfilled'
      const statusSucceeded = statusResult.status === 'fulfilled'
      let fetchedNotice = ''
      if (noticeSucceeded && noticeResult.value.success === true) {
        fetchedNotice = (noticeResult.value.data || '').trim()
      }
      const fetchedStatus = statusSucceeded ? statusResult.value : null
      const timelineEnabled = fetchedStatus?.announcements_enabled === true
      const fetchedAnnouncements = timelineEnabled
        ? ((fetchedStatus?.announcements || []) as AnnouncementItem[]).slice(
            0,
            20
          )
        : []

      const previous = latestSnapshotRef.current
      const snapshot: NotificationSnapshot = {
        notice: noticeSucceeded ? fetchedNotice : (previous?.notice ?? ''),
        noticeVersion: noticeSucceeded ? fetchedNotice : null,
        announcements: statusSucceeded
          ? fetchedAnnouncements
          : (previous?.announcements ?? []),
        announcementsVersion:
          statusSucceeded && timelineEnabled
            ? getAnnouncementsVersion(fetchedAnnouncements)
            : null,
      }

      if (noticeSucceeded || statusSucceeded) {
        setLatest(snapshot)

        const currentStore = useNotificationStore.getState()
        const noticeUpdated =
          noticeSucceeded &&
          hasUnconfirmedNotice(snapshot, currentStore.confirmedNoticeVersion)
        const announcementsUpdated =
          statusSucceeded &&
          timelineEnabled &&
          hasUnconfirmedAnnouncements(
            snapshot,
            currentStore.confirmedAnnouncementsVersion
          )

        if (
          isAuthenticated &&
          !isDialogOpenRef.current &&
          (noticeUpdated || announcementsUpdated)
        ) {
          let nextTab = activeTabRef.current
          if (noticeUpdated && !announcementsUpdated) {
            nextTab = 'notice'
          } else if (announcementsUpdated && !noticeUpdated) {
            nextTab = 'announcements'
          }
          setTab(nextTab)
          setDialogSnapshot(snapshot)
          isDialogOpenRef.current = true
          setPopoverOpen(true)
        }
      }
    })().finally(() => {
      if (checkPromiseRef.current === checkPromise) {
        checkPromiseRef.current = null
      }
    })

    checkPromiseRef.current = checkPromise
    return checkPromise
  }, [
    enabled,
    isAuthenticated,
    queryClient,
    setDialogSnapshot,
    setLatest,
    setTab,
    userId,
  ])

  useEffect(() => {
    isMountedRef.current = true
    isDialogOpenRef.current = false
    setPopoverOpen(false)
    setDialogSnapshot(null)
    latestSnapshotRef.current = null
    setLatestSnapshot(null)
    void checkForUpdates()

    const checkWhenVisible = () => {
      if (document.visibilityState === 'visible') void checkForUpdates()
    }
    window.addEventListener('focus', checkWhenVisible)
    document.addEventListener('visibilitychange', checkWhenVisible)
    return () => {
      isMountedRef.current = false
      checkGenerationRef.current += 1
      checkPromiseRef.current = null
      window.removeEventListener('focus', checkWhenVisible)
      document.removeEventListener('visibilitychange', checkWhenVisible)
    }
  }, [checkForUpdates, setDialogSnapshot])

  useEffect(() => {
    // Manual opening can happen before the first requests finish. Capture
    // their settled result once, then keep it frozen until the dialog closes.
    if (
      !popoverOpen ||
      dialogSnapshot !== null ||
      noticeQuery.isFetching ||
      statusQuery.isFetching ||
      noticeQuery.isPending ||
      statusQuery.isPending
    ) {
      return
    }
    setDialogSnapshot(currentQuerySnapshot)
  }, [
    currentQuerySnapshot,
    dialogSnapshot,
    noticeQuery.isFetching,
    noticeQuery.isPending,
    popoverOpen,
    setDialogSnapshot,
    statusQuery.isFetching,
    statusQuery.isPending,
  ])

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        const pending = noticeQuery.isFetching || statusQuery.isFetching
        setDialogSnapshot(pending ? null : currentQuerySnapshot)
        isDialogOpenRef.current = true
        setPopoverOpen(true)
        return
      }

      const snapshot = dialogSnapshotRef.current
      if (snapshot) {
        useNotificationStore
          .getState()
          .confirmVersions(
            snapshot.noticeVersion,
            snapshot.announcementsVersion
          )
      }
      isDialogOpenRef.current = false
      setPopoverOpen(false)
      setDialogSnapshot(null)
    },
    [
      currentQuerySnapshot,
      noticeQuery.isFetching,
      setDialogSnapshot,
      statusQuery.isFetching,
    ]
  )

  const latest = latestSnapshot ?? currentQuerySnapshot
  const unreadNotice = hasUnconfirmedNotice(latest, confirmedNoticeVersion)
  const unreadAnnouncements = hasUnconfirmedAnnouncements(
    latest,
    confirmedAnnouncementsVersion
  )
  const shownSnapshot = dialogSnapshot ?? currentQuerySnapshot

  return {
    notice: shownSnapshot.notice,
    announcements: shownSnapshot.announcements,
    loading:
      dialogSnapshot === null &&
      (noticeQuery.isFetching ||
        statusQuery.isFetching ||
        noticeQuery.isPending ||
        statusQuery.isPending),
    unreadCount: Number(unreadNotice) + Number(unreadAnnouncements),
    noticeUpdated: hasUnconfirmedNotice(shownSnapshot, confirmedNoticeVersion),
    announcementsUpdated: hasUnconfirmedAnnouncements(
      shownSnapshot,
      confirmedAnnouncementsVersion
    ),
    popoverOpen,
    setPopoverOpen: handleOpenChange,
    activeTab,
    setActiveTab: setTab,
    openPopover: () => handleOpenChange(true),
    closePopover: () => handleOpenChange(false),
    refetchNotice: noticeQuery.refetch,
  }
}

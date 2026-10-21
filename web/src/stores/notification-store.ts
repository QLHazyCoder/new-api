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
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface NotificationState {
  confirmedNoticeVersion: string | null
  confirmedAnnouncementsVersion: string | null

  confirmVersions: (
    noticeVersion: string | null,
    announcementsVersion: string | null
  ) => void
}

type LegacyNotificationState = {
  lastReadNotice?: unknown
}

/**
 * Persists the exact content versions confirmed by closing the notification
 * dialog. Legacy timeline IDs cannot establish which content was confirmed,
 * so existing timeline content needs one confirmation after this upgrade.
 */
export const useNotificationStore = create<NotificationState>()(
  persist(
    (set) => ({
      confirmedNoticeVersion: null,
      confirmedAnnouncementsVersion: null,

      confirmVersions: (noticeVersion, announcementsVersion) => {
        set((state) => ({
          confirmedNoticeVersion: noticeVersion ?? state.confirmedNoticeVersion,
          confirmedAnnouncementsVersion:
            announcementsVersion ?? state.confirmedAnnouncementsVersion,
        }))
      },
    }),
    {
      name: 'notification-storage',
      version: 2,
      migrate: (persistedState, version) => {
        if (version >= 2) {
          return persistedState as NotificationState
        }

        const legacyState = persistedState as LegacyNotificationState | null
        return {
          confirmedNoticeVersion:
            typeof legacyState?.lastReadNotice === 'string'
              ? legacyState.lastReadNotice.trim()
              : null,
          confirmedAnnouncementsVersion: null,
        }
      },
      partialize: (state) => ({
        confirmedNoticeVersion: state.confirmedNoticeVersion,
        confirmedAnnouncementsVersion: state.confirmedAnnouncementsVersion,
      }),
    }
  )
)

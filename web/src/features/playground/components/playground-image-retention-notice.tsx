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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import {
  loadImageRetentionNoticeDismissed,
  saveImageRetentionNoticeDismissed,
} from '../lib/storage/storage'

export function PlaygroundImageRetentionNotice({ open }: { open: boolean }) {
  const { t } = useTranslation()
  const [noticeOpen, setNoticeOpen] = useState(false)

  useEffect(() => {
    if (open && !loadImageRetentionNoticeDismissed()) {
      setNoticeOpen(true)
    } else {
      setNoticeOpen(false)
    }
  }, [open])

  return (
    <Dialog open={noticeOpen} onOpenChange={setNoticeOpen}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Image storage and retention notice')}</DialogTitle>
          <DialogDescription>
            {t(
              'Images generated in the playground are stored on the server for a limited time: at most 200 images are kept per account and they are automatically deleted after 1 day. Please download important images in time.'
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type='button'
            variant='ghost'
            onClick={() => {
              saveImageRetentionNoticeDismissed(true)
              setNoticeOpen(false)
            }}
          >
            {t("Don't show again")}
          </Button>
          <Button type='button' onClick={() => setNoticeOpen(false)}>
            {t('Got it')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

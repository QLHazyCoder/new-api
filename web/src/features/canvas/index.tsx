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

import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { useAuthStore } from '@/stores/auth-store'

import { createCanvasBridge } from './bridge'

export function Canvas() {
  const { t } = useTranslation()
  const iframe = useRef<HTMLIFrameElement>(null)
  const [version, setVersion] = useState(0)
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>(
    'loading'
  )
  const userID = useAuthStore((state) => state.auth.user?.id)
  const sid = useAuthStore((state) => state.auth.session?.sid)

  useEffect(() => {
    setStatus('loading')
    if (!userID || !sid) return
    const frame = iframe.current
    if (!frame) return
    let dispose: (() => void) | undefined
    let channel: MessageChannel | undefined
    const timeout = window.setTimeout(() => setStatus('failed'), 30_000)
    const initialize = () => {
      dispose?.()
      channel?.port1.close()
      channel = new MessageChannel()
      const activeChannel = channel
      const nonce = crypto.randomUUID()
      channel.port1.onmessage = (event: MessageEvent) => {
        if (event.data?.type !== 'ready' || event.data.nonce !== nonce) return
        window.clearTimeout(timeout)
        setStatus('ready')
        dispose = createCanvasBridge(
          activeChannel.port1,
          () => setStatus('failed'),
          () => {
            const current = useAuthStore.getState().auth
            return current.user?.id === userID && current.session?.sid === sid
          }
        )
      }
      channel.port1.start()
      frame.contentWindow?.postMessage(
        { type: 'canvas:init', protocol: 1, nonce },
        '*',
        [channel.port2]
      )
    }
    frame.addEventListener('load', initialize)
    // This effect installs the listener before assigning src.
    frame.src = '/canvas-app/index.html#/canvas'
    return () => {
      frame.removeEventListener('load', initialize)
      window.clearTimeout(timeout)
      dispose?.()
      channel?.port1.close()
      frame.src = 'about:blank'
    }
  }, [userID, sid, version])

  return (
    <section className='relative flex h-full min-h-0 flex-1 flex-col overflow-hidden'>
      {status === 'loading' && <LoadingState className='absolute inset-0' />}
      {status === 'failed' && (
        <ErrorState
          title={t('Infinite Canvas')}
          description={t('Unable to open canvas')}
          onRetry={() => setVersion((value) => value + 1)}
        />
      )}
      <iframe
        key={`${String(userID)}:${String(sid)}`}
        ref={iframe}
        title={t('Infinite Canvas')}
        sandbox='allow-scripts allow-downloads'
        className={
          status === 'failed'
            ? 'hidden'
            : 'h-full min-h-0 w-full flex-1 border-0'
        }
        referrerPolicy='same-origin'
        aria-busy={status === 'loading'}
      />
    </section>
  )
}

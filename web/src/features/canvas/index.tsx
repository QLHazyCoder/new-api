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
import { useTheme } from '@/context/theme-provider'
import { getBuildRevision } from '@/lib/build-metadata'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { createCanvasBridge } from './bridge'

export function Canvas() {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const iframe = useRef<HTMLIFrameElement>(null)
  const themeRef = useRef<'light' | 'dark'>(resolvedTheme)
  const portRef = useRef<MessagePort | undefined>(undefined)
  const [version, setVersion] = useState(0)
  const [status, setStatus] = useState<'loading' | 'ready' | 'failed'>(
    'loading'
  )
  const userID = useAuthStore((state) => state.auth.user?.id)
  const sid = useAuthStore((state) => state.auth.session?.sid)

  useEffect(() => {
    themeRef.current = resolvedTheme
    portRef.current?.postMessage({ type: 'theme', theme: resolvedTheme })
  }, [resolvedTheme])

  useEffect(() => {
    setStatus('loading')
    if (!userID || !sid) return
    const frame = iframe.current
    if (!frame) return
    let active = true
    let dispose: (() => void) | undefined
    let channel: MessageChannel | undefined
    const fail = () => {
      if (!active) return
      active = false
      window.clearTimeout(timeout)
      dispose?.()
      channel?.port1.close()
      portRef.current = undefined
      frame.src = 'about:blank'
      setStatus('failed')
    }
    // A connected channel does not mean that the application has loaded.
    // Keep this deadline until its first successful render.
    const timeout = window.setTimeout(fail, 30_000)
    const initialize = () => {
      if (!active) return
      dispose?.()
      channel?.port1.close()
      channel = new MessageChannel()
      const activeChannel = channel
      const nonce = crypto.randomUUID()
      channel.port1.onmessage = (event: MessageEvent) => {
        if (
          !active ||
          channel !== activeChannel ||
          event.data?.type !== 'ready' ||
          event.data.nonce !== nonce
        ) {
          return
        }
        dispose = createCanvasBridge(
          activeChannel.port1,
          fail,
          () => {
            const current = useAuthStore.getState().auth
            return (
              active &&
              channel === activeChannel &&
              current.user?.id === userID &&
              current.session?.sid === sid
            )
          },
          () => {
            window.clearTimeout(timeout)
            setStatus('ready')
          }
        )
        portRef.current = activeChannel.port1
        activeChannel.port1.postMessage({
          type: 'theme',
          theme: themeRef.current,
        })
      }
      channel.port1.start()
      frame.contentWindow?.postMessage(
        { type: 'canvas:init', protocol: 1, nonce, theme: themeRef.current },
        '*',
        [channel.port2]
      )
    }
    frame.addEventListener('load', initialize)
    // This effect installs the listener before assigning src.
    // Older releases cached a permanent redirect for the unversioned entry.
    // A new build must navigate past that cache before it can read new headers.
    frame.src = `/canvas-app/index.html?v=${encodeURIComponent(getBuildRevision())}#/canvas`
    return () => {
      active = false
      frame.removeEventListener('load', initialize)
      window.clearTimeout(timeout)
      dispose?.()
      channel?.port1.close()
      portRef.current = undefined
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
        className={cn(
          'h-full min-h-0 w-full flex-1 border-0',
          status === 'loading' && 'invisible',
          status === 'failed' && 'hidden'
        )}
        referrerPolicy='same-origin'
        aria-busy={status === 'loading'}
      />
    </section>
  )
}

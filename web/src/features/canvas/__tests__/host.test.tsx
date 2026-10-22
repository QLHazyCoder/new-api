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

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { Canvas } from '..'

const channels: Array<{
  port1: {
    onmessage: ((event: MessageEvent) => void) | null
    start: ReturnType<typeof vi.fn>
    close: ReturnType<typeof vi.fn>
    postMessage: ReturnType<typeof vi.fn>
  }
  port2: object
}> = []

function signIn(id: number) {
  useAuthStore.getState().auth.setBundle({
    access_token: `session-secret-${id}`,
    access_expires_at: 9999999999,
    token_type: 'Bearer',
    user: { id, username: 'canvas-user', role: 1 },
    session: {
      sid: `session-${id}`,
      current: true,
      login_method: 'password',
      ip: '',
      user_agent: '',
      created_at: 1,
      last_active_at: 1,
      expires_at: 9999999999,
    },
  })
}

beforeEach(() => {
  channels.length = 0
  vi.stubGlobal(
    'MessageChannel',
    class {
      port1 = {
        onmessage: null,
        start: vi.fn(),
        close: vi.fn(),
        postMessage: vi.fn(),
      }
      port2 = {}
      constructor() {
        channels.push(this)
      }
    }
  )
  signIn(1)
})
afterEach(() => {
  cleanup()
  useAuthStore.getState().auth.reset()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('Canvas host lifecycle', () => {
  it('keeps loading until the connected child has rendered its application', () => {
    render(<Canvas />)
    const frame = screen.getByTitle('Infinite Canvas') as HTMLIFrameElement
    const frameWindow = frame.contentWindow
    if (!frameWindow) throw new Error('Iframe window is unavailable')
    const postMessage = vi.spyOn(frameWindow, 'postMessage')
    fireEvent.load(frame)
    const initialization = postMessage.mock.calls.at(-1)?.[0] as {
      nonce: string
    }
    const port = channels.at(-1)?.port1
    expect(port).toBeDefined()
    act(() => {
      port?.onmessage?.(
        new MessageEvent('message', {
          data: { type: 'ready', nonce: initialization.nonce },
        })
      )
    })
    expect(frame.getAttribute('aria-busy')).toBe('true')
    expect(frame.classList.contains('invisible')).toBe(true)
    act(() => {
      port?.onmessage?.(
        new MessageEvent('message', { data: { type: 'loaded' } })
      )
    })
    expect(frame.getAttribute('aria-busy')).toBe('false')
    expect(frame.classList.contains('invisible')).toBe(false)
    expect(screen.queryByText('Loading...')).toBeNull()
  })
  it('shows a retry when initialization stalls after the channel connects', () => {
    vi.useFakeTimers()
    render(<Canvas />)
    const frame = screen.getByTitle('Infinite Canvas') as HTMLIFrameElement
    const frameWindow = frame.contentWindow
    if (!frameWindow) throw new Error('Iframe window is unavailable')
    const postMessage = vi.spyOn(frameWindow, 'postMessage')
    fireEvent.load(frame)
    const initialization = postMessage.mock.calls.at(-1)?.[0] as {
      nonce: string
    }
    const port = channels.at(-1)?.port1
    expect(port).toBeDefined()
    act(() => {
      port?.onmessage?.(
        new MessageEvent('message', {
          data: { type: 'ready', nonce: initialization.nonce },
        })
      )
      vi.advanceTimersByTime(30_000)
    })
    expect(screen.getByText('Unable to open canvas')).toBeTruthy()
    expect(port?.close).toHaveBeenCalled()
    act(() => {
      port?.onmessage?.(
        new MessageEvent('message', { data: { type: 'loaded' } })
      )
    })
    expect(screen.getByText('Unable to open canvas')).toBeTruthy()
  })
  it('restricts the iframe and closes the former account channel on account switch', () => {
    render(<Canvas />)
    const frame = screen.getByTitle('Infinite Canvas') as HTMLIFrameElement
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts allow-downloads')
    const frameURL = new URL(frame.src)
    expect(frameURL.pathname).toBe('/canvas-app/index.html')
    expect(frameURL.searchParams.get('v')).toBeTruthy()
    expect(frameURL.hash).toBe('#/canvas')
    fireEvent.load(frame)
    const active = channels.at(-1)
    expect(active).toBeDefined()
    act(() => signIn(2))
    expect(active?.port1.close).toHaveBeenCalled()
    expect(screen.getByTitle('Infinite Canvas')).not.toBe(frame)
    expect(document.body.textContent).not.toContain('session-secret')
  })
  it('shows a retry action when the child fails to complete the handshake', () => {
    vi.useFakeTimers()
    render(<Canvas />)
    act(() => vi.advanceTimersByTime(30_000))
    expect(screen.getByText('Unable to open canvas')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.queryByText('Unable to open canvas')).toBeNull()
    expect(screen.getByTitle('Infinite Canvas').getAttribute('aria-busy')).toBe(
      'true'
    )
  })
})

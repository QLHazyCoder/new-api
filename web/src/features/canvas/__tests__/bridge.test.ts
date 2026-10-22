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

import { describe, expect, it, vi, afterEach } from 'vitest'

import { canvasEndpoint, createCanvasBridge } from '../bridge'

vi.mock('@/lib/auth-session', () => ({
  getFreshAuthHeaders: async () => ({
    Authorization: 'Bearer session-canary',
    'Content-Type': 'application/json',
  }),
}))

afterEach(() => vi.unstubAllGlobals())

describe('Canvas bridge operations', () => {
  it('rejects arbitrary targets and path traversal', () => {
    expect(() =>
      canvasEndpoint('proxy', {
        url: 'https://other.test',
        Authorization: 'secret',
      })
    ).toThrow()
    expect(() =>
      canvasEndpoint('assets.content', { id: '../other-user' })
    ).toThrow()
    expect(() =>
      canvasEndpoint('state.get', { namespace: 'users', key: 'admin' })
    ).toThrow()
  })
  it('constructs the image edit form and keeps auth out of the payload', () => {
    const target = canvasEndpoint('images.edit', {
      group: 'images',
      form: [
        ['model', 'image-model'],
        ['image', new Blob(['image'], { type: 'image/png' }), 'ref.png'],
      ],
    })
    expect(target.path).toBe('/v1/images/edits')
    expect(target.group).toBe('images')
    expect(target.body).toBeInstanceOf(FormData)
    expect((target.body as FormData).get('model')).toBe('image-model')
    expect(target).not.toHaveProperty('Authorization')
  })
  it('streams bytes with backpressure and never sends session auth to the child', async () => {
    const first = new TextEncoder().encode('中文🙂')
    let receive: (event: MessageEvent) => Promise<void>
    const replies: unknown[] = []
    const send = vi.fn((message: { type: string; id: string }) => {
      replies.push(message)
      if (message.type === 'chunk') {
        queueMicrotask(
          () =>
            void receive({
              data: { type: 'ack', id: message.id },
            } as MessageEvent)
        )
      }
    })
    const port = {
      postMessage: send,
      start: vi.fn(),
      close: vi.fn(),
      onmessage: undefined,
      onmessageerror: undefined,
    } as unknown as MessagePort
    const fetcher = vi.fn().mockResolvedValue(
      new Response(first, {
        headers: { 'Content-Type': 'text/event-stream' },
      })
    )
    vi.stubGlobal('fetch', fetcher)
    const dispose = createCanvasBridge(port, vi.fn())
    receive = port.onmessage as unknown as typeof receive
    await receive({
      data: {
        type: 'request',
        protocol: 1,
        id: 'request-1',
        operation: 'responses.create',
        payload: { body: { model: 'model', input: '中文' } },
      },
    } as MessageEvent)
    expect(fetcher).toHaveBeenCalledOnce()
    expect(fetcher.mock.calls[0][0]).toBe('/api/canvas/v1/responses')
    expect(fetcher.mock.calls[0][1].headers.get('Authorization')).toBe(
      'Bearer session-canary'
    )
    expect(JSON.stringify(replies)).not.toContain('session-canary')
    const chunks = replies as Array<{ type: string; data?: ArrayBuffer }>
    expect(
      new TextDecoder().decode(
        chunks.find((reply) => reply.type === 'chunk')?.data
      )
    ).toBe('中文🙂')
    expect(chunks.at(-1)?.type).toBe('end')
    await receive({
      data: {
        type: 'request',
        protocol: 1,
        id: 'request-1',
        operation: 'responses.create',
        payload: {},
      },
    } as MessageEvent)
    expect(fetcher).toHaveBeenCalledOnce()
    dispose()
    expect(port.close).toHaveBeenCalledOnce()
  })
  it('ignores late stream messages after the child cancels a response body', async () => {
    vi.resetModules()
    vi.spyOn(document, 'referrer', 'get').mockReturnValue(
      'https://console.test/canvas'
    )
    const child =
      await import('../../../../../integrations/infinite-canvas/overlay/bridge')
    const send = vi.fn()
    const port = {
      postMessage: send,
      start: vi.fn(),
      onmessage: null,
    } as unknown as MessagePort
    window.dispatchEvent(
      new MessageEvent('message', {
        source: window.parent,
        origin: 'https://console.test',
        data: { type: 'canvas:init', protocol: 1, nonce: 'test' },
        ports: [port],
      })
    )
    await child.ready
    const result = child.canvasRequest('assets.content', { id: 'image:one' })
    await vi.waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'request' })
      )
    )
    const request = send.mock.calls.find(
      ([message]) => message.type === 'request'
    )?.[0]
    const receive = (data: unknown) =>
      port.onmessage?.call(port, { data } as MessageEvent)
    receive({ type: 'response', id: request.id, status: 200, headers: {} })
    const response = await result
    await response.body!.cancel()
    expect(send).toHaveBeenCalledWith({ type: 'cancel', id: request.id })
    expect(() => receive({ type: 'end', id: request.id })).not.toThrow()
  })
})

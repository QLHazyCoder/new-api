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

import { getFreshAuthHeaders } from '@/lib/auth-session'

export type CanvasRequest = {
  protocol: 1
  type: 'request'
  id: string
  operation: string
  payload: Record<string, unknown>
}

const idPattern = /^[\w:-]{1,192}$/
const namespaces = new Set([
  'app_state',
  'image_generation_logs',
  'video_generation_logs',
  'prompt_cache',
])

function resource(payload: Record<string, unknown>, key = 'id'): string {
  const value = payload[key]
  if (typeof value !== 'string' || !idPattern.test(value)) {
    throw new Error('Invalid canvas content')
  }
  return encodeURIComponent(value)
}

export function canvasEndpoint(
  operation: string,
  payload: Record<string, unknown>
): { path: string; method: string; body?: BodyInit; group?: string } {
  const json = (value: unknown): string => JSON.stringify(value)
  switch (operation) {
    case 'bootstrap':
      return { path: '/bootstrap', method: 'GET' }
    case 'projects.list':
      return {
        path: `/projects?offset=${Number(payload.offset ?? 0)}${payload.trash === true ? '&trash=true' : ''}`,
        method: 'GET',
      }
    case 'projects.get':
      return { path: `/projects/${resource(payload)}`, method: 'GET' }
    case 'projects.save':
      return {
        path:
          Number(payload.revision) === 0
            ? '/projects'
            : `/projects/${resource(payload)}`,
        method: Number(payload.revision) === 0 ? 'POST' : 'PATCH',
        body: json({
          project_id: payload.id,
          revision: payload.revision,
          document: payload.document,
        }),
      }
    case 'projects.delete':
      return {
        path: `/projects/${resource(payload)}?revision=${Number(
          payload.revision
        )}`,
        method: 'DELETE',
      }
    case 'projects.restore':
      return {
        path: `/projects/${resource(payload)}/restore`,
        method: 'POST',
      }
    case 'assets.list':
      return {
        path: `/assets?offset=${Number(payload.offset ?? 0)}`,
        method: 'GET',
      }
    case 'assets.upload':
      if (!(payload.blob instanceof Blob)) {
        throw new Error('Invalid canvas content')
      }
      return {
        path: `/assets/${resource(payload)}/content`,
        method: 'PUT',
        body: payload.blob,
      }
    case 'assets.content':
      return {
        path: `/assets/${resource(payload)}/content`,
        method: 'GET',
      }
    case 'assets.delete':
      return { path: `/assets/${resource(payload)}`, method: 'DELETE' }
    case 'assets.import':
      return { path: '/assets/import', method: 'POST', body: json(payload) }
    case 'state.list':
    case 'state.get':
    case 'state.set':
    case 'state.delete': {
      if (!namespaces.has(String(payload.namespace))) {
        throw new Error('Invalid canvas content')
      }
      const path = `/state/${payload.namespace}`
      if (operation === 'state.list') return { path, method: 'GET' }
      const itemPath = `${path}/${resource(payload, 'key')}`
      if (operation === 'state.get') return { path: itemPath, method: 'GET' }
      if (operation === 'state.delete') {
        return {
          path: `${itemPath}?revision=${Number(payload.revision)}`,
          method: 'DELETE',
        }
      }
      return {
        path: itemPath,
        method: 'PUT',
        body: json({ revision: payload.revision, value: payload.value }),
      }
    }
    case 'runs.list':
      return {
        path: `/runs?offset=${Number(payload.offset ?? 0)}`,
        method: 'GET',
      }
    case 'runs.get':
      return { path: `/runs/${resource(payload)}`, method: 'GET' }
    case 'runs.result':
      return { path: `/runs/${resource(payload)}/result`, method: 'GET' }
    case 'runs.cancel':
      return { path: `/runs/${resource(payload)}/cancel`, method: 'POST' }
    case 'videos.retrieve':
      return { path: `/v1/videos/${resource(payload)}`, method: 'GET' }
    case 'videos.content':
      return {
        path: `/v1/videos/${resource(payload)}/content`,
        method: 'GET',
      }
    case 'responses.retrieve':
      return { path: `/v1/responses/${resource(payload)}`, method: 'GET' }
    case 'artifacts.list':
      return { path: `/v1/tasks/${resource(payload)}/artifacts`, method: 'GET' }
    case 'artifacts.content':
      return {
        path: `/v1/tasks/${resource(payload)}/artifacts/${resource(
          payload,
          'key'
        )}/content`,
        method: 'GET',
      }
    default:
      break
  }
  const paths: Record<string, string> = {
    'responses.create': '/v1/responses',
    'images.generate': '/v1/images/generations',
    'images.edit': '/v1/images/edits',
    'videos.create': '/v1/videos',
    'audio.speech': '/v1/audio/speech',
  }
  const path = paths[operation]
  if (!path) throw new Error('Invalid canvas content')
  let body: BodyInit
  if (Array.isArray(payload.form)) {
    const form = new FormData()
    for (const part of payload.form as unknown[]) {
      if (!Array.isArray(part) || typeof part[0] !== 'string') {
        throw new Error('Invalid canvas content')
      }
      if (part[1] instanceof Blob) {
        form.append(part[0], part[1], String(part[2] ?? 'media'))
      } else if (typeof part[1] === 'string') form.append(part[0], part[1])
      else throw new Error('Invalid canvas content')
    }
    body = form
  } else {
    body = json(payload.body)
  }
  return { path, method: 'POST', body, group: String(payload.group ?? '') }
}

// The channel exposes fixed business operations only. Authentication headers
// are created here and never included in a message sent to the iframe.
export function createCanvasBridge(
  port: MessagePort,
  closed: () => void,
  isCurrent: () => boolean = () => true,
  loaded: () => void = () => undefined
): () => void {
  const requests = new Map<string, AbortController>()
  const acknowledgements = new Map<string, () => void>()
  const usedIDs = new Set<string>()
  let disposed = false
  port.onmessage = async (
    event: MessageEvent<
      | CanvasRequest
      | { type: 'cancel' | 'ack'; id: string }
      | { type: 'fatal' | 'loaded' }
    >
  ) => {
    const message = event.data
    if (!message || disposed || !isCurrent()) return
    if (message.type === 'loaded') {
      loaded()
      return
    }
    if (message.type === 'fatal') {
      closed()
      return
    }
    if (message.type === 'ack') {
      acknowledgements.get(message.id)?.()
      return
    }
    if (message.type === 'cancel') {
      requests.get(message.id)?.abort()
      return
    }
    if (
      message.type !== 'request' ||
      message.protocol !== 1 ||
      typeof message.id !== 'string' ||
      !idPattern.test(message.id) ||
      usedIDs.has(message.id) ||
      !message.payload ||
      typeof message.payload !== 'object'
    ) {
      return
    }
    usedIDs.add(message.id)
    if (requests.size >= 16) {
      port.postMessage({
        type: 'error',
        id: message.id,
        message: 'Too many requests',
      })
      return
    }
    const abort = new AbortController()
    requests.set(message.id, abort)
    try {
      const target = canvasEndpoint(message.operation, message.payload)
      const headers = new Headers(await getFreshAuthHeaders())
      if (disposed || !isCurrent()) return
      if (target.body instanceof FormData) headers.delete('Content-Type')
      if (target.body instanceof Blob) {
        headers.set('Content-Type', target.body.type)
      }
      if (target.group) headers.set('X-Canvas-Group', target.group)
      if (target.method === 'POST' && target.path.startsWith('/v1/')) {
        headers.set('X-Canvas-Request-ID', message.id)
      }
      const response = await fetch(`/api/canvas${target.path}`, {
        method: target.method,
        body: target.body,
        headers,
        signal: abort.signal,
        credentials: 'same-origin',
        cache: 'no-store',
      })
      if (disposed || !isCurrent()) return
      port.postMessage({
        type: 'response',
        id: message.id,
        status: response.status,
        headers: {
          'content-type': response.headers.get('content-type') ?? '',
          'x-canvas-run-id': response.headers.get('x-canvas-run-id') ?? '',
        },
      })
      if (response.body) {
        const reader = response.body.getReader()
        try {
          while (true) {
            const chunk = await reader.read()
            if (disposed || !isCurrent()) return
            if (chunk.done) break
            // Send transferable bytes for both SSE and media, without decoding
            // UTF-8 at chunk boundaries.
            const data = chunk.value.buffer.slice(
              chunk.value.byteOffset,
              chunk.value.byteOffset + chunk.value.byteLength
            )
            await new Promise<void>((resolve, reject) => {
              const cancelled = () => {
                acknowledgements.delete(message.id)
                reject(new DOMException('Aborted', 'AbortError'))
              }
              abort.signal.addEventListener('abort', cancelled, { once: true })
              acknowledgements.set(message.id, () => {
                acknowledgements.delete(message.id)
                abort.signal.removeEventListener('abort', cancelled)
                resolve()
              })
              port.postMessage({ type: 'chunk', id: message.id, data }, [data])
            })
          }
        } finally {
          reader.releaseLock()
        }
      }
      if (!disposed && isCurrent()) {
        port.postMessage({ type: 'end', id: message.id })
      }
    } catch (error) {
      if (!disposed && isCurrent()) {
        port.postMessage({
          type: 'error',
          id: message.id,
          message: error instanceof Error ? error.message : 'Request failed',
        })
      }
    } finally {
      requests.delete(message.id)
      acknowledgements.delete(message.id)
    }
  }
  port.onmessageerror = closed
  port.start()
  return () => {
    disposed = true
    for (const controller of requests.values()) controller.abort()
    requests.clear()
    usedIDs.clear()
    port.close()
  }
}

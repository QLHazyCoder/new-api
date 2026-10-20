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
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
} from 'vitest'

import { api } from '@/lib/api'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { DocsArticlePage } from '../docs-article'
import { DocsIndexPage } from '../docs-index'
import { DocsLayout } from '../docs-layout'
import type { PublicDocArticle, PublicDocNode } from '../types'

const docs: PublicDocArticle[] = [
  {
    slug: 'quick-start',
    title: 'Quick Start',
    markdown: 'Published introduction',
    updated_at: '2026-10-20T09:00:00Z',
    path: ['Guides'],
  },
  {
    slug: 'api-reference',
    title: 'API Reference',
    markdown: 'Published API reference',
    updated_at: '2026-10-20T10:00:00Z',
    path: ['Guides'],
  },
]
const tree: PublicDocNode[] = [
  {
    type: 'folder',
    id: 1,
    name: 'Guides',
    sort_order: 0,
    children: docs.map((article, index) => ({
      type: 'article',
      id: index + 1,
      slug: article.slug,
      title: article.title,
      sort_order: index * 10,
    })),
  },
]
type MockableApi = { get: (url: string) => Promise<{ data: unknown }> }
const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const clients: QueryClient[] = []
const requests: string[] = []
let publishedTree: PublicDocNode[] | null = tree

const animations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})
afterAll(() => {
  if (animations) {
    Object.defineProperty(HTMLElement.prototype, 'getAnimations', animations)
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

beforeEach(() => {
  publishedTree = tree
  requests.length = 0
  window.localStorage.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setConfig({ systemName: 'Coder API' })
  useSystemConfigStore.getState().setLoading(false)
  apiClient.get = async (url) => {
    requests.push(url)
    if (url === '/api/public/docs') {
      return { data: { success: true, data: publishedTree } }
    }
    const article = docs.find((item) => url === `/api/public/docs/${item.slug}`)
    if (article) return { data: { success: true, data: article } }
    throw Object.assign(new Error('Missing published article'), {
      response: { status: 404 },
    })
  }
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  apiClient.get = originalGet
  window.localStorage.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
})

async function openDocs(path = '/docs') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(STATUS_QUERY_KEY, {
    docs_link: `${window.location.origin}/docs`,
    HeaderNavModules: { home: true, console: true, docs: true },
  })
  client.setQueryData(['notice'], { success: true, data: '' })
  const root = createRootRoute({ component: Outlet })
  const docsRoute = createRoute({
    getParentRoute: () => root,
    path: 'docs',
    component: () => (
      <DocsLayout>
        <Outlet />
      </DocsLayout>
    ),
  })
  const index = createRoute({
    getParentRoute: () => docsRoute,
    path: '/',
    component: DocsIndexPage,
  })
  const article = createRoute({
    getParentRoute: () => docsRoute,
    path: '$slug',
    component: () => <DocsArticlePage slug={article.useParams().slug} />,
  })
  const router = createRouter({
    routeTree: root.addChildren([docsRoute.addChildren([index, article])]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { router, view }
}

test('anonymous root navigation selects the first published article and keeps the site header', async () => {
  const { router } = await openDocs()
  expect(
    await screen.findByRole('heading', { name: 'Quick Start' })
  ).toBeInTheDocument()
  expect(router.state.location.pathname).toBe('/docs/quick-start')
  expect(screen.getByRole('banner')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Coder API/ })).toBeInTheDocument()
  for (const link of screen.getAllByRole('link', { name: 'Docs' })) {
    expect(link).toHaveAttribute('href', '/docs')
    expect(link).not.toHaveAttribute('target', '_blank')
  }
  expect(requests).toEqual(['/api/public/docs', '/api/public/docs/quick-start'])
})

test('directory search and article selection keep the header and update the internal route', async () => {
  const { router } = await openDocs('/docs/quick-start')
  await screen.findByRole('heading', { name: 'Quick Start' })
  await userEvent.type(
    screen.getByRole('searchbox', { name: 'Search documentation' }),
    'reference'
  )
  await userEvent.click(screen.getByRole('button', { name: 'API Reference' }))
  expect(
    await screen.findByRole('heading', { name: 'API Reference' })
  ).toBeInTheDocument()
  expect(router.state.location.pathname).toBe('/docs/api-reference')
  expect(screen.getByRole('banner')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'API Reference' })).toHaveAttribute(
    'aria-current',
    'page'
  )
})

test('deep links to unpublished articles show a missing-document state inside the site layout', async () => {
  await openDocs('/docs/unpublished')
  expect(await screen.findByText('Document not found')).toBeInTheDocument()
  expect(screen.getByRole('banner')).toBeInTheDocument()
  expect(screen.queryByRole('article')).toBeNull()
})

test('an empty published tree shows the empty state and leaves the root address unchanged', async () => {
  publishedTree = null
  const { router } = await openDocs()
  await waitFor(() =>
    expect(
      screen.getAllByText('No published documentation yet').length
    ).toBeGreaterThan(0)
  )
  expect(router.state.location.pathname).toBe('/docs')
  expect(requests).toEqual(['/api/public/docs'])
})

test('the mobile directory closes after article selection and restores the reading layout', async () => {
  const { router } = await openDocs('/docs/quick-start')
  await screen.findByRole('heading', { name: 'Quick Start' })
  await userEvent.click(
    screen.getByRole('button', { name: 'Open documentation directory' })
  )
  const drawer = await screen.findByRole('dialog', {
    name: 'Documentation directory',
  })
  await userEvent.click(
    within(drawer).getByRole('button', { name: 'API Reference' })
  )
  expect(
    await screen.findByRole('heading', { name: 'API Reference' })
  ).toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Documentation directory' })
    ).toBeNull()
  )
  expect(router.state.location.pathname).toBe('/docs/api-reference')
  expect(
    screen.getByRole('button', { name: 'Open documentation directory' })
  ).toHaveAttribute('aria-expanded', 'false')
})

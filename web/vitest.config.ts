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
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineConfig } from 'vitest/config'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    server: {
      deps: { inline: [/@lobehub\//, /antd-style/] },
    },
    setupFiles: ['./src/test-setup.ts'],
    // Heavy jsdom suites render full drawers, editors, and data tables. Their
    // cold render can be substantially slower when Vitest workers share a CI
    // host, so keep a bounded 30s budget rather than letting the default 5s
    // turn scheduler contention into false failures.
    testTimeout: 30000,
    // Keep heavy jsdom files from saturating the runner. A percentage scales
    // down on small CI runners while capping this host at four workers.
    maxWorkers: '40%',
    clearMocks: true,
    restoreMocks: true,
    include: [
      'src/**/*.{test,spec}.{ts,tsx}',
      'scripts/oxlint/__tests__/*.test.ts',
    ],
  },
})

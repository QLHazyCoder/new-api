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

export type PerformanceCounterSource = {
  request_count?: number | null
  success_count?: number | null
  success_rate?: number | null
}

export type PerformanceCounterTotals = {
  requestCount: number
  successCount: number
  failedCount: number
}

function finiteNonNegative(value: unknown): number | null {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 0) {
    return null
  }
  return parsed
}

export function sumPerformanceCounters(
  rows: PerformanceCounterSource[]
): PerformanceCounterTotals {
  let requestCount = 0
  let successCount = 0

  for (const row of rows) {
    const rowRequests = finiteNonNegative(row.request_count)
    const rowSuccesses = finiteNonNegative(row.success_count)
    if (rowRequests === null || rowSuccesses === null || rowRequests <= 0) {
      continue
    }
    requestCount += rowRequests
    successCount += Math.min(rowSuccesses, rowRequests)
  }

  return {
    requestCount,
    successCount,
    failedCount: Math.max(0, requestCount - successCount),
  }
}

export function weightedSuccessRate(
  rows: PerformanceCounterSource[]
): number {
  const totals = sumPerformanceCounters(rows)
  if (totals.requestCount > 0) {
    return (totals.successCount / totals.requestCount) * 100
  }

  let total = 0
  let count = 0
  for (const row of rows) {
    const value = Number(row.success_rate)
    if (!Number.isFinite(value)) {
      continue
    }
    total += value
    count++
  }

  return count > 0 ? total / count : Number.NaN
}

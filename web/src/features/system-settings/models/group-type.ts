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
import { AUTO_GROUP_TYPE_ID } from '@/lib/group-types'

import { safeJsonParse } from '../utils/json-parser'

export type GroupTypeDefinition = {
  id: string
  name: string
  icon: string
  order: number
}

export type GroupTypeMap = Record<string, string>

let groupTypeIdCounter = 0

function sortGroupTypes(types: GroupTypeDefinition[]): GroupTypeDefinition[] {
  return [...types].sort((a, b) => {
    if (a.order !== b.order) return a.order - b.order
    const nameOrder = a.name.localeCompare(b.name)
    return nameOrder !== 0 ? nameOrder : a.id.localeCompare(b.id)
  })
}

export function parseGroupTypes(value: string): GroupTypeDefinition[] {
  const parsed = safeJsonParse<Record<string, unknown>>(value, {
    fallback: {},
    silent: true,
    context: 'group types',
  })
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return []

  const types: GroupTypeDefinition[] = []
  for (const [id, rawType] of Object.entries(parsed)) {
    const typeID = id.trim()
    if (!typeID || typeID === AUTO_GROUP_TYPE_ID) continue
    if (!rawType || typeof rawType !== 'object' || Array.isArray(rawType)) {
      continue
    }
    const type = rawType as Record<string, unknown>
    if (typeof type.name !== 'string' || !type.name.trim()) continue
    types.push({
      id: typeID,
      name: type.name.trim(),
      icon: typeof type.icon === 'string' ? type.icon.trim() : '',
      order:
        typeof type.order === 'number' && Number.isFinite(type.order)
          ? type.order
          : 0,
    })
  }
  return sortGroupTypes(types)
}

export function serializeGroupTypes(types: GroupTypeDefinition[]): string {
  const serialized: Record<
    string,
    { name: string; icon: string; order: number }
  > = {}
  for (const [index, type] of sortGroupTypes(types).entries()) {
    if (
      !type.id.trim() ||
      type.id.trim() === AUTO_GROUP_TYPE_ID ||
      !type.name.trim()
    ) {
      continue
    }
    serialized[type.id.trim()] = {
      name: type.name.trim(),
      icon: type.icon.trim(),
      order: index,
    }
  }
  return JSON.stringify(serialized, null, 2)
}

export function parseGroupTypeMap(value: string): GroupTypeMap {
  const parsed = safeJsonParse<Record<string, unknown>>(value, {
    fallback: {},
    silent: true,
    context: 'group type assignments',
  })
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}

  const assignments: GroupTypeMap = {}
  for (const [groupName, typeID] of Object.entries(parsed)) {
    const name = groupName.trim()
    if (
      !name ||
      name === 'auto' ||
      typeof typeID !== 'string' ||
      !typeID.trim()
    ) {
      continue
    }
    assignments[name] = typeID.trim()
  }
  return assignments
}

export function serializeGroupTypeMap(
  assignments: GroupTypeMap,
  validTypeIDs: ReadonlySet<string>,
  validGroupNames?: ReadonlySet<string>
): string {
  const serialized: GroupTypeMap = {}
  for (const groupName of Object.keys(assignments).sort()) {
    const typeID = assignments[groupName]?.trim()
    if (
      !groupName.trim() ||
      groupName === 'auto' ||
      (validGroupNames && !validGroupNames.has(groupName)) ||
      !typeID ||
      typeID === AUTO_GROUP_TYPE_ID ||
      !validTypeIDs.has(typeID)
    ) {
      continue
    }
    serialized[groupName] = typeID
  }
  return JSON.stringify(serialized, null, 2)
}

export function createGroupTypeId(existingIDs: ReadonlySet<string>): string {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    groupTypeIdCounter += 1
    const randomID =
      typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
        ? crypto.randomUUID()
        : `type_${Date.now().toString(36)}_${groupTypeIdCounter}`
    if (!existingIDs.has(randomID)) return randomID
  }
  return `type_${Date.now().toString(36)}_${groupTypeIdCounter}`
}

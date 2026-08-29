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
import { EXCLUDED_GROUPS, FILTER_ALL, QUOTA_TYPE_VALUES } from '../constants'
import type { PriceType, PricingModel } from '../types'

// ----------------------------------------------------------------------------
// Model Helper Utilities
// ----------------------------------------------------------------------------

/**
 * Get available groups for a model
 */
export function getAvailableGroups(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>
): string[] {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  return Object.keys(usableGroup)
    .filter((g) => !EXCLUDED_GROUPS.includes(g))
    .filter((g) => modelEnableGroups.includes(g))
}

/**
 * Read a configured group ratio while preserving valid zero ratios.
 */
export function getConfiguredGroupRatio(
  groupRatio: Record<string, number>,
  group: string
): number {
  const ratio = groupRatio[group]
  return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : 1
}

export function getEffectiveModelGroupRatio(
  model: PricingModel,
  groupRatio: Record<string, number>,
  group: string
): number {
  const modelRatio = model.model_group_ratio
  if (typeof modelRatio === 'number' && Number.isFinite(modelRatio)) {
    return modelRatio
  }
  return getConfiguredGroupRatio(groupRatio, group)
}

export function getEffectiveModelItemGroupRatio(
  model: PricingModel,
  type: PriceType,
  fallback: number
): number {
  let ratio: number | undefined
  switch (type) {
    case 'output':
    case 'audio_output':
      ratio = model.model_completion_group_ratio
      break
    case 'cache':
      ratio = model.model_cache_group_ratio
      break
    case 'create_cache':
      ratio = model.model_create_cache_group_ratio
      break
    default:
      ratio = undefined
  }
  return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : fallback
}

/**
 * Build the pricing projection used for upstream/base-price displays.
 * Model-level selling discounts must not leak into the original-price view.
 */
export function getOriginalPricingModel(model: PricingModel): PricingModel {
  const groups = Array.isArray(model.enable_groups) ? model.enable_groups : []
  return {
    ...model,
    model_group_ratio: undefined,
    model_completion_group_ratio: undefined,
    model_cache_group_ratio: undefined,
    model_create_cache_group_ratio: undefined,
    group_ratio: Object.fromEntries(groups.map((group) => [group, 1])),
  }
}

/**
 * Resolve the group ratio used by model square summary prices.
 *
 * When no specific group is selected, the model square shows the best price
 * available to the viewer. When a group filter is active, it shows that
 * group's price instead.
 */
export function getDisplayGroupRatio(
  model: PricingModel,
  selectedGroup?: string
): number {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []
  const groupRatio = model.group_ratio || {}

  if (
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    modelEnableGroups.includes(selectedGroup)
  ) {
    return getEffectiveModelGroupRatio(model, groupRatio, selectedGroup)
  }

  if (modelEnableGroups.length === 0) {
    return 1
  }

  let minRatio = Number.POSITIVE_INFINITY

  for (const group of modelEnableGroups) {
    const ratio = getEffectiveModelGroupRatio(model, groupRatio, group)
    if (
      typeof ratio === 'number' &&
      Number.isFinite(ratio) &&
      ratio < minRatio
    ) {
      minRatio = ratio
    }
  }

  return minRatio === Number.POSITIVE_INFINITY ? 1 : minRatio
}

/**
 * Replace model placeholder in endpoint path
 */
export function replaceModelInPath(path: string, modelName: string): string {
  return path.replaceAll('{model}', modelName)
}

/**
 * Check if model is token-based pricing
 */
export function isTokenBasedModel(model: PricingModel): boolean {
  return model.quota_type === QUOTA_TYPE_VALUES.TOKEN
}

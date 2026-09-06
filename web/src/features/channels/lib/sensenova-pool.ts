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
import type { KeyStatus } from '../types'

export const SENSENOVA_BASE_URL = 'https://token.sensenova.cn'
export const SENSENOVA_MODELS: string[] = [
  'deepseek-v4-pro',
  'deepseek-v4-flash',
  'glm-5.2',
  'kimi-k3',
]

export function getSenseNovaStatus(key: KeyStatus): string {
  if (key.status === 2) return 'Manual Disabled'
  if (key.status !== 1) return 'Auto Disabled'
  switch (key.health?.state) {
    case 'usable':
      return 'Usable'
    case 'cooling':
      return 'Cooling'
    case 'invalid':
      return 'Invalid credential'
    default:
      return 'Untested'
  }
}

export function canProbeSenseNovaKey(
  key: KeyStatus,
  canEditSensitive: boolean
): boolean {
  return (
    canEditSensitive &&
    key.status === 1 &&
    Boolean(key.key_id) &&
    key.health?.state !== 'invalid'
  )
}

export function getSenseNovaReasonLabel(reason?: string): string {
  switch (reason) {
    case 'quota_exhausted':
      return 'Upstream quota exhausted; reset time unknown'
    case 'authentication_failed':
      return 'Upstream authentication failed'
    case 'rate_limited':
      return 'Upstream rate limit; quota exhaustion unconfirmed'
    case 'model_unavailable':
      return 'Model temporarily unavailable'
    case 'upstream_unavailable':
      return 'Upstream temporarily unavailable'
    case 'probe_failed':
      return 'Health probe did not succeed'
    case 'upstream_request_rejected':
      return 'Upstream request rejected'
    default:
      return reason ? 'Unknown' : '-'
  }
}

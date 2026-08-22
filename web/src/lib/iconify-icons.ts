/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { addIcon } from '@iconify/react/offline'

// 本地 Iconify 图标集合：避免关键界面依赖运行时 CDN/API。
const ICONS = {
  activity:
    '<path fill="currentColor" d="M3 12h4l2-8 4 16 2-8h6v2h-7l-1 4h-2L9 11l-1 3H3z"/>',
  arrowUpRight:
    '<path fill="currentColor" d="M5 4v2h11.59L4 18.59 5.41 20 18 7.41V18h2V4z"/>',
  book: '<path fill="currentColor" d="M5 3h11a3 3 0 0 1 3 3v14H7a4 4 0 0 1-4-4V6a3 3 0 0 1 3-3m0 2a1 1 0 0 0-1 1v11c0 1.1.9 2 2 2h11V6a1 1 0 0 0-1-1zm2 3h7v2H7zm0 4h7v2H7z"/>',
  command:
    '<path fill="currentColor" d="M4 4h6v2H6v4H4zm10 0h6v6h-2V6h-4zM4 14h2v4h4v2H4zm14 0h2v6h-6v-2h4z"/>',
  plus: '<path fill="currentColor" d="M11 5h2v6h6v2h-6v6h-2v-6H5v-2h6z"/>',
  layers:
    '<path fill="currentColor" d="m12 3 9 5-9 5-9-5zm0 12 7-3.89L21 12l-9 5-9-5 2-1.11zm0 5 7-3.89L21 17l-9 5-9-5 2-1.11z"/>',
  route:
    '<path fill="currentColor" d="M6 3a3 3 0 1 1 0 6 3 3 0 0 1 0-6m0 2a1 1 0 1 0 0 2 1 1 0 0 0 0-2m12 10a3 3 0 1 1 0 6 3 3 0 0 1 0-6m0 2a1 1 0 1 0 0 2 1 1 0 0 0 0-2M8 5h6a4 4 0 0 1 4 4v5h-2V9a2 2 0 0 0-2-2H8z"/>',
  shield:
    '<path fill="currentColor" d="M12 2 4 5v6c0 5.25 3.41 9.74 8 11 4.59-1.26 8-5.75 8-11V5zm0 2.18 6 2.25V11c0 4.06-2.5 7.73-6 9-3.5-1.27-6-4.94-6-9V6.43z"/>',
  settings:
    '<path fill="currentColor" d="m19.43 12.98 1.77 1.38-2 3.46-2.1-.88a7.1 7.1 0 0 1-1.7.98L15.1 20h-4l-.3-2.08a7.1 7.1 0 0 1-1.7-.98l-2.1.88-2-3.46 1.77-1.38a6.7 6.7 0 0 1 0-1.96L5 9.62l2-3.46 2.1.88a7.1 7.1 0 0 1 1.7-.98L11.1 4h4l.3 2.08a7.1 7.1 0 0 1 1.7.98l2.1-.88 2 3.46-1.77 1.38a6.7 6.7 0 0 1 0 1.96M13.1 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7"/>',
  terminal:
    '<path fill="currentColor" d="M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2m0 2v12h16V6zm2 2 5 4-5 4-1.25-1.56L7.8 12l-3.05-2.44zM12 15h5v2h-5z"/>',
} as const

for (const [name, body] of Object.entries(ICONS)) {
  addIcon(`ak47:${name}`, { body, width: 24, height: 24 })
}

export type IconifyName = keyof typeof ICONS

export function iconifyName(name: IconifyName) {
  return `ak47:${name}`
}

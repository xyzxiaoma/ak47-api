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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { Window } from 'happy-dom'
import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import type { PricingModel } from '../../types'
import { ModelCardGrid } from '../model-card-grid'

test('models in the same group are all visible as separate cards', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const models: PricingModel[] = [
    'deepseek-v4-pro',
    'deepseek-v4-flash',
    'glm-5.2',
    'kimi-k3',
  ].map((model_name, id) => ({
    id,
    model_name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 2,
    enable_groups: ['shared'],
    model_group_ratio: 0.1,
  }))
  const dom = new Window()
  try {
    dom.document.body.innerHTML = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ModelCardGrid models={models} onModelClick={() => {}} />
      </I18nextProvider>
    )
    const headings = [...dom.document.querySelectorAll('h3')]
    assert.deepEqual(
      headings.map((heading) => heading.textContent),
      models.map((model) => model.model_name)
    )
    for (const heading of headings) {
      assert.equal(heading.closest('[inert], [aria-hidden="true"]'), null)
    }
    assert.equal(dom.document.querySelectorAll('ul > li').length, 4)
  } finally {
    await dom.happyDOM.close()
  }
})

test('empty model results render no cards or pagination', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  assert.equal(
    renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <ModelCardGrid models={[]} onModelClick={() => {}} />
      </I18nextProvider>
    ),
    ''
  )
})

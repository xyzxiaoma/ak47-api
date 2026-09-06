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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { ChannelFormValues } from '../../lib/channel-form'
import type { MultiKeyConfirmAction } from '../../types'

const domWindow = new Window()
domWindow.document.write(
  '<!doctype html><html><head></head><body></body></html>'
)
const globals = [
  'window',
  'document',
  'navigator',
  'localStorage',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
  'matchMedia',
  'customElements',
  'CSSStyleSheet',
  'ShadowRoot',
  'Document',
  'DocumentFragment',
] as const
const descriptors = new Map<string, PropertyDescriptor | undefined>()
for (const key of globals) {
  descriptors.set(key, Object.getOwnPropertyDescriptor(globalThis, key))
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value:
      key === 'matchMedia'
        ? domWindow.matchMedia.bind(domWindow)
        : domWindow[key],
  })
}
descriptors.set(
  'IS_REACT_ACT_ENVIRONMENT',
  Object.getOwnPropertyDescriptor(globalThis, 'IS_REACT_ACT_ENVIRONMENT')
)
Object.defineProperty(globalThis, 'IS_REACT_ACT_ENVIRONMENT', {
  configurable: true,
  value: true,
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { useForm } = await import('react-hook-form')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { Form } = await import('@/components/ui/form')
const { SenseNovaPoolField } =
  await import('../drawers/sections/sensenova-pool-field')
const { SenseNovaKeyHealth } = await import('../dialogs/sensenova-key-health')
const { MultiKeyTableRowActions } =
  await import('../dialogs/multi-key-table-row-actions')
const { CHANNEL_FORM_DEFAULT_VALUES } = await import('../../lib/channel-form')
const { SENSENOVA_MODELS } = await import('../../lib/sensenova-pool')
const { BalanceCell } = await import('../channels-columns')
const { ChannelsProvider } = await import('../channels-provider')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { channelSchema } = await import('../../types')
const { api } = await import('@/lib/api')

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })

function PresetHarness(props: { disabled?: boolean }) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      base_url: 'https://existing.example',
      models: 'existing-model',
      key: 'fixture-key',
    },
  })
  const values = form.watch()
  return (
    <I18nextProvider i18n={i18n}>
      <Form {...form}>
        <SenseNovaPoolField form={form} disabled={props.disabled ?? false} />
        <output aria-label='Configuration'>
          {JSON.stringify({
            url: values.base_url,
            models: values.models,
            enabled: values.sensenova_pool,
            mode: values.multi_key_mode,
          })}
        </output>
      </Form>
    </I18nextProvider>
  )
}

describe('SenseNova operator controls', () => {
  after(() => {
    domWindow.close()
    for (const [key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor)
      else Reflect.deleteProperty(globalThis, key)
    }
  })

  test('pool balance displays N/A and clicking it never queries an unsupported balance endpoint', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const adapter = api.defaults.adapter
    let requests = 0
    api.defaults.adapter = async (config) => {
      requests++
      return {
        data: { success: true },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    const channel = channelSchema.parse({
      id: 1,
      type: 1,
      key: '',
      status: 1,
      name: 'Pool',
      sensenova_pool: true,
      balance: 987654321,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
    })
    try {
      await act(async () =>
        root.render(
          <I18nextProvider i18n={i18n}>
            <QueryClientProvider client={queryClient}>
              <ChannelsProvider>
                <BalanceCell channel={channel} />
              </ChannelsProvider>
            </QueryClientProvider>
          </I18nextProvider>
        )
      )
      const remaining = [
        ...container.querySelectorAll<HTMLElement>('span'),
      ].find((element) => element.textContent === 'N/A')
      assert.ok(remaining)
      await act(async () => remaining.click())
      assert.equal(requests, 0)
      assert.equal(container.textContent?.includes('987'), false)
    } finally {
      await act(async () => root.unmount())
      queryClient.clear()
      api.defaults.adapter = adapter
      container.remove()
    }
  })

  test('opening the form preserves existing values and selecting the preset configures the shared pool', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(<PresetHarness />))
      const output = container.querySelector('output')
      assert.ok(output)
      assert.deepEqual(JSON.parse(output.textContent || '{}'), {
        url: 'https://existing.example',
        models: 'existing-model',
        enabled: false,
        mode: 'single',
      })
      const toggle = container.querySelector<HTMLElement>('[role="switch"]')
      assert.ok(toggle)
      await act(async () => toggle.click())
      assert.equal(toggle.getAttribute('aria-checked'), 'true')
      assert.deepEqual(JSON.parse(output.textContent || '{}'), {
        url: 'https://token.sensenova.cn',
        models: SENSENOVA_MODELS.join(','),
        enabled: true,
        mode: 'multi_to_single',
      })
      await act(async () => toggle.click())
      assert.equal(JSON.parse(output.textContent || '{}').enabled, false)
      assert.equal(
        JSON.parse(output.textContent || '{}').models,
        SENSENOVA_MODELS.join(',')
      )
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('sensitive permission lock prevents applying the preset', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(<PresetHarness disabled />))
      const toggle = container.querySelector<HTMLElement>('[role="switch"]')
      assert.ok(toggle)
      assert.equal(
        toggle.hasAttribute('disabled') ||
          toggle.getAttribute('aria-disabled') === 'true',
        true
      )
      await act(async () => toggle.click())
      assert.equal(toggle.getAttribute('aria-checked'), 'false')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('manual probe action carries the stable identifier and unauthorized probing stays disabled', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    let action: MultiKeyConfirmAction | undefined
    const onAction = (value: MultiKeyConfirmAction) => {
      action = value
    }
    try {
      await act(async () =>
        root.render(
          <I18nextProvider i18n={i18n}>
            <MultiKeyTableRowActions
              keyIndex={2}
              keyId='opaque-id'
              status={1}
              canDelete
              showTest
              canTest
              onAction={onAction}
            />
          </I18nextProvider>
        )
      )
      const probe = [...container.querySelectorAll('button')].find(
        (button) => button.textContent === 'Test key'
      )
      assert.ok(probe)
      await act(async () => probe.click())
      assert.deepEqual(action, {
        type: 'test',
        keyIndex: 2,
        keyId: 'opaque-id',
      })
      action = undefined
      await act(async () =>
        root.render(
          <I18nextProvider i18n={i18n}>
            <MultiKeyTableRowActions
              keyIndex={2}
              keyId='opaque-id'
              status={1}
              canDelete={false}
              showTest
              canTest={false}
              onAction={onAction}
            />
          </I18nextProvider>
        )
      )
      assert.equal(probe.disabled, true)
      await act(async () => probe.click())
      assert.equal(action, undefined)
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })

  test('manually disabled key shows its override and suppresses a stale future probe time', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () =>
        root.render(
          <I18nextProvider i18n={i18n}>
            <SenseNovaKeyHealth
              entry={{
                index: 0,
                status: 2,
                reason: 'operator pause',
                health: {
                  state: 'usable',
                  reason: '',
                  last_success_at: 0,
                  last_failure_at: 0,
                  last_probe_at: 0,
                  next_probe_at: 1800000000,
                },
              }}
            />
          </I18nextProvider>
        )
      )
      assert.ok(container.textContent?.includes('Manual Disabled'))
      assert.ok(container.textContent?.includes('operator pause'))
      const nextProbe = [...container.querySelectorAll('dt')].find(
        (term) => term.textContent === 'Next probe'
      )
      assert.equal(nextProbe?.nextElementSibling?.textContent, '-')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})

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
import { describe, test } from 'node:test'

import { channelSchema } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  transformChannelToFormDefaults,
} from '../channel-form'
import {
  canProbeSenseNovaKey,
  getSenseNovaStatus,
  SENSENOVA_BASE_URL,
  SENSENOVA_MODELS,
} from '../sensenova-pool'

const poolForm = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Independent account pool',
  sensenova_pool: true,
  base_url: SENSENOVA_BASE_URL,
  models: SENSENOVA_MODELS.join(','),
  key: 'fixture-account-a\nfixture-account-b',
  multi_key_mode: 'multi_to_single' as const,
  multi_key_type: 'polling' as const,
}

describe('SenseNova channel form contract', () => {
  test('opted-in pool creates one polling channel and preserves the flag on update', () => {
    assert.equal(channelFormSchema.safeParse(poolForm).success, true)
    const created = transformFormDataToCreatePayload(poolForm)
    assert.equal(created.mode, 'multi_to_single')
    assert.equal(created.multi_key_mode, 'polling')
    assert.equal(created.channel.sensenova_pool, true)
    assert.equal(created.channel.models, SENSENOVA_MODELS.join(','))
    assert.equal(
      transformFormDataToUpdatePayload(poolForm, 3).sensenova_pool,
      true
    )
  })

  test('saving an existing pool does not require reentering secrets or change its models', () => {
    const channel = channelSchema.parse({
      id: 3,
      type: 1,
      key: '',
      status: 1,
      name: 'Existing pool',
      sensenova_pool: true,
      base_url: SENSENOVA_BASE_URL,
      models: 'glm-5.2',
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      channel_info: { is_multi_key: true, multi_key_mode: 'polling' },
    })
    const defaults = transformChannelToFormDefaults(channel)
    assert.equal(defaults.models, 'glm-5.2')
    assert.equal(defaults.key, '')
    assert.equal(channelFormSchema.safeParse(defaults).success, true)
    assert.equal(transformFormDataToUpdatePayload(defaults, 3).key, undefined)
  })

  test('unsupported model, upstream or selection policy cannot be submitted as a SenseNova pool', () => {
    for (const change of [
      { models: 'sensenova-6.8-flash-lite' },
      { base_url: 'https://another-upstream.example' },
      { type: 14 },
      { multi_key_mode: 'batch' as const },
      { multi_key_type: 'random' as const },
    ]) {
      assert.equal(
        channelFormSchema.safeParse({ ...poolForm, ...change }).success,
        false
      )
    }
  })

  test('unrelated channels keep their existing upstream and do not enable the pool', () => {
    const form = {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Other',
      models: 'other-model',
      base_url: 'https://other.example',
    }
    assert.equal(channelFormSchema.safeParse(form).success, true)
    assert.equal(
      transformFormDataToCreatePayload(form).channel.sensenova_pool,
      false
    )
  })

  test('pool overrides are rejected on their fields instead of silently discarded', () => {
    for (const field of [
      'model_mapping',
      'status_code_mapping',
      'header_override',
      'param_override',
    ] as const) {
      const override =
        field === 'status_code_mapping'
          ? '{"429":"200"}'
          : '{"value":"override"}'
      const result = channelFormSchema.safeParse({
        ...poolForm,
        [field]: override,
      })
      assert.equal(result.success, false)
      if (!result.success) {
        assert.ok(
          result.error.issues.some(
            (issue) =>
              issue.path[0] === field &&
              issue.message ===
                'SenseNova pool does not support model, status, header or parameter overrides'
          )
        )
      }
      assert.equal(
        channelFormSchema.safeParse({ ...poolForm, [field]: '{}' }).success,
        true
      )
    }
  })
})

describe('SenseNova visible state and probe eligibility', () => {
  test('new keys are untested and manual disable takes priority over a successful probe', () => {
    assert.equal(getSenseNovaStatus({ index: 0, status: 1 }), 'Untested')
    const key = {
      index: 0,
      status: 2,
      key_id: 'stable-id',
      health: {
        state: 'usable' as const,
        reason: '',
        last_success_at: 1,
        last_failure_at: 0,
        last_probe_at: 1,
        next_probe_at: 0,
      },
    }
    assert.equal(getSenseNovaStatus(key), 'Manual Disabled')
    assert.equal(canProbeSenseNovaKey(key, true), false)
  })

  test('only authorized enabled keys with stable identity can be probed', () => {
    const key = { index: 0, status: 1, key_id: 'stable-id' }
    assert.equal(canProbeSenseNovaKey(key, true), true)
    assert.equal(canProbeSenseNovaKey(key, false), false)
    assert.equal(
      canProbeSenseNovaKey({ ...key, key_id: undefined }, true),
      false
    )
    assert.equal(
      canProbeSenseNovaKey(
        {
          ...key,
          health: {
            state: 'invalid',
            reason: 'invalid_credentials',
            last_success_at: 0,
            last_failure_at: 1,
            last_probe_at: 0,
            next_probe_at: 0,
          },
        },
        true
      ),
      false
    )
  })
})

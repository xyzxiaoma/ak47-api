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
import { useTranslation } from 'react-i18next'

import { formatTimestamp } from '../../lib'
import {
  getSenseNovaStatus,
  getSenseNovaReasonLabel,
} from '../../lib/sensenova-pool'
import type { KeyStatus } from '../../types'

export function SenseNovaKeyHealth(props: { entry: KeyStatus }) {
  const { t } = useTranslation()
  const health = props.entry.health
  const timestamps = [
    { label: t('Last success'), value: health?.last_success_at },
    { label: t('Last failure'), value: health?.last_failure_at },
    { label: t('Last probe'), value: health?.last_probe_at },
    {
      label: t('Next probe'),
      value: props.entry.status === 1 ? health?.next_probe_at : 0,
    },
  ]
  return (
    <div className='min-w-60 space-y-1 text-sm'>
      <p className='font-medium'>{t(getSenseNovaStatus(props.entry))}</p>
      <p className='text-muted-foreground break-words'>
        {props.entry.status === 2
          ? props.entry.reason || '-'
          : t(getSenseNovaReasonLabel(health?.reason))}
      </p>
      <dl className='text-muted-foreground grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
        {timestamps.map((entry) => (
          <div key={entry.label} className='contents'>
            <dt>{entry.label}</dt>
            <dd>{entry.value ? formatTimestamp(entry.value) : '-'}</dd>
          </div>
        ))}
      </dl>
      {health?.model_states?.map((model) => (
        <p key={model.model} className='text-xs break-words'>
          {model.model}:{' '}
          {model.state === 'invalid' ? t('Invalid credential') : t('Cooling')} ·{' '}
          {t(getSenseNovaReasonLabel(model.reason))}
          {model.next_probe_at > 0 && props.entry.status === 1 && (
            <>
              {' '}
              · {t('Next probe')}: {formatTimestamp(model.next_probe_at)}
            </>
          )}
        </p>
      ))}
    </div>
  )
}

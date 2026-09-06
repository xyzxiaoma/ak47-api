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
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../../../lib/channel-form'
import {
  SENSENOVA_BASE_URL,
  SENSENOVA_MODELS,
} from '../../../lib/sensenova-pool'

export function SenseNovaPoolField(props: {
  form: UseFormReturn<ChannelFormValues>
  disabled: boolean
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={props.form.control}
      name='sensenova_pool'
      render={({ field }) => (
        <FormItem className='rounded-md border p-3'>
          <div className='flex items-center justify-between gap-4'>
            <FormLabel>{t('SenseNova key pool')}</FormLabel>
            <FormControl>
              <Switch
                checked={field.value ?? false}
                disabled={props.disabled}
                onCheckedChange={(checked) => {
                  field.onChange(checked)
                  if (!checked) return
                  props.form.setValue('type', 1, { shouldDirty: true })
                  props.form.setValue('base_url', SENSENOVA_BASE_URL, {
                    shouldDirty: true,
                  })
                  props.form.setValue('models', SENSENOVA_MODELS.join(','), {
                    shouldDirty: true,
                  })
                  props.form.setValue('multi_key_mode', 'multi_to_single', {
                    shouldDirty: true,
                  })
                  props.form.setValue('multi_key_type', 'polling', {
                    shouldDirty: true,
                  })
                  props.form.setValue(
                    'upstream_model_update_auto_sync_enabled',
                    false,
                    { shouldDirty: true }
                  )
                }}
              />
            </FormControl>
          </div>
          <FormDescription>
            {t(
              'Enabling sets the SenseNova URL, OpenAI type, four supported models and multi-key polling. Paste one independent account key per line in the key editor.'
            )}
          </FormDescription>
          <FormDescription>
            {t(
              'These four models share each account’s general pool. API keys cannot reveal remaining credits or exact reset times; failed keys cool down and return after a successful probe.'
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

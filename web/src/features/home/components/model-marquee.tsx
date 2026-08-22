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
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { usePricingData } from '@/features/pricing/hooks'

const fallbackModels = [
  'OpenAI',
  'Claude',
  'Gemini',
  'DeepSeek',
  'Qwen',
  'Llama',
]

export function ModelMarquee() {
  const { t } = useTranslation()
  const { models } = usePricingData()
  const modelNames = useMemo(() => {
    const names = models.map((model) => model.model_name.trim()).filter(Boolean)
    return [...new Set(names)].slice(0, 24)
  }, [models])
  const visibleModels = modelNames.length > 0 ? modelNames : fallbackModels

  return (
    <section
      className='landing-model-marquee border-y border-[color:var(--border)]/70'
      aria-label={t('AI models supported')}
    >
      <div className='mx-auto flex max-w-[90rem] items-center gap-6 px-5 py-4 sm:px-8 lg:px-12'>
        <div className='landing-mono-label shrink-0'>{t('AI models')}</div>
        <div className='landing-model-track-wrap min-w-0 flex-1'>
          <div className='landing-model-track'>
            {[0, 1].map((setIndex) =>
              visibleModels.map((name) => (
                <Link
                  key={`${setIndex}-${name}`}
                  to='/pricing'
                  className='landing-model-pill'
                  aria-hidden={setIndex === 1}
                  tabIndex={setIndex === 1 ? -1 : undefined}
                >
                  <span
                    className='signal-dot signal-dot-ember'
                    aria-hidden='true'
                  />
                  {name}
                </Link>
              ))
            )}
          </div>
        </div>
        <Link
          to='/pricing'
          className='landing-model-marquee-link hidden shrink-0 sm:inline-flex'
        >
          {t('View all currently available models')}
        </Link>
      </div>
    </section>
  )
}

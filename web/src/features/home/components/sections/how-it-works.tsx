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
import { Icon } from '@iconify/react/offline'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { iconifyName } from '@/lib/iconify-icons'

export function HowItWorks() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '01',
      title: t('Configure'),
      desc: t(
        'Add your API keys, set up channels and configure access permissions'
      ),
      icon: 'command' as const,
    },
    {
      num: '02',
      title: t('Connect'),
      desc: t(
        'Connect through OpenAI, Claude, Gemini, and other compatible API routes'
      ),
      icon: 'route' as const,
    },
    {
      num: '03',
      title: t('Monitor'),
      desc: t('Track usage, costs and performance with real-time analytics'),
      icon: 'activity' as const,
    },
  ]

  return (
    <section className='landing-section border-t border-[color:var(--border)]/80 px-5 py-24 sm:px-8 md:py-32 lg:px-12'>
      <div className='mx-auto grid max-w-[90rem] gap-12 lg:grid-cols-[.52fr_1.48fr] lg:gap-24'>
        <AnimateInView>
          <p className='landing-mono-label mb-4'>{t('How It Works')}</p>
          <h2 className='landing-section-title'>
            {t('Three steps to get started')}
          </h2>
          <p className='landing-body-copy mt-6 max-w-xs'>
            {t('Track usage, costs and performance with real-time analytics')}
          </p>
        </AnimateInView>

        <div className='relative'>
          <div className='landing-step-spine' aria-hidden='true' />
          <div className='space-y-4'>
            {steps.map((step, index) => (
              <AnimateInView
                key={step.num}
                delay={index * 110}
                animation='fade-up'
                className='landing-step-row'
              >
                <div className='landing-step-number'>{step.num}</div>
                <div className='landing-step-icon'>
                  <Icon
                    icon={iconifyName(step.icon)}
                    width='20'
                    height='20'
                    aria-hidden='true'
                  />
                </div>
                <div className='min-w-0 flex-1'>
                  <h3 className='text-lg font-semibold tracking-tight'>
                    {step.title}
                  </h3>
                  <p className='landing-body-copy mt-1 max-w-xl text-sm'>
                    {step.desc}
                  </p>
                </div>
              </AnimateInView>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

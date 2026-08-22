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

interface FeaturesProps {
  className?: string
}

const featureData = [
  {
    title: 'High Performance',
    desc: 'Support for high concurrency with automatic load balancing',
    icon: 'activity' as const,
    accent: 'ember',
  },
  {
    title: 'Transparent Billing',
    desc: 'Pay-as-you-go with real-time usage monitoring',
    icon: 'layers' as const,
    accent: 'moss',
  },
  {
    title: 'Team Collaboration',
    desc: 'Multi-user management with flexible permission allocation',
    icon: 'shield' as const,
    accent: 'sand',
  },
  {
    title: 'Open Source',
    desc: 'Community driven, self-hosted, and extensible',
    icon: 'terminal' as const,
    accent: 'rust',
  },
]

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  return (
    <section className='landing-section relative px-5 py-24 sm:px-8 md:py-32 lg:px-12'>
      <div className='mx-auto grid max-w-[90rem] gap-12 lg:grid-cols-[.72fr_1.28fr] lg:gap-24'>
        <AnimateInView className='max-w-md self-start lg:sticky lg:top-28'>
          <p className='landing-mono-label mb-4'>{t('Core Features')}</p>
          <h2 className='landing-section-title'>
            {t('Built for developers,')}
            <span>{t('designed for scale')}</span>
          </h2>
          <p className='landing-body-copy mt-6'>
            {t('Compatible API routes for common AI application workflows')}
          </p>
          <div className='text-muted-foreground mt-10 flex items-center gap-3 text-xs'>
            <span className='landing-rule-number'>02</span>
            <span className='h-px w-16 bg-[color:var(--border)]' />
            <span>{t('Core Features')}</span>
          </div>
        </AnimateInView>

        <div className='landing-feature-stack'>
          <AnimateInView
            className='landing-feature-ledger landing-feature-ledger-primary'
            animation='fade-up'
          >
            <div className='flex items-start justify-between gap-5'>
              <div>
                <span className='landing-feature-index'>01</span>
                <h3 className='mt-7 text-2xl font-semibold tracking-tight'>
                  {t('Lightning Fast')}
                </h3>
                <p className='landing-body-copy mt-3 max-w-sm'>
                  {t(
                    'Optimized network architecture ensures millisecond response times'
                  )}
                </p>
              </div>
              <Icon
                icon={iconifyName('route')}
                width='27'
                height='27'
                className='text-[color:var(--brand-ember)]'
                aria-hidden='true'
              />
            </div>
            <div className='mt-8 flex flex-wrap gap-2'>
              {['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'Llama'].map(
                (name) => (
                  <span key={name} className='landing-model-tag'>
                    {name}
                  </span>
                )
              )}
            </div>
          </AnimateInView>

          <div className='grid gap-px overflow-hidden border border-[color:var(--border)]/80 bg-[color:var(--border)]/70 sm:grid-cols-2'>
            {featureData.map((feature, index) => (
              <AnimateInView
                key={feature.title}
                delay={index * 90}
                animation='fade-up'
                className={`landing-feature-ledger landing-feature-ledger-${feature.accent}`}
              >
                <div className='flex items-center justify-between gap-4'>
                  <span className='landing-feature-index'>0{index + 2}</span>
                  <Icon
                    icon={iconifyName(feature.icon)}
                    width='22'
                    height='22'
                    aria-hidden='true'
                  />
                </div>
                <h3 className='mt-10 text-base font-semibold'>
                  {t(feature.title)}
                </h3>
                <p className='landing-body-copy mt-2 text-sm'>
                  {t(feature.desc)}
                </p>
              </AnimateInView>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'
import { iconifyName } from '@/lib/iconify-icons'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  if (props.isAuthenticated) return null

  return (
    <section className='landing-section px-5 py-24 sm:px-8 md:py-32 lg:px-12'>
      <AnimateInView className='landing-cta-panel mx-auto grid max-w-[90rem] gap-10 p-7 sm:p-10 lg:grid-cols-[1fr_auto] lg:items-end lg:p-14'>
        <div>
          <div className='landing-kicker'>
            <span className='signal-dot signal-dot-ember' />
            04 / 04
          </div>
          <h2 className='landing-section-title mt-5 max-w-2xl'>
            {t('Ready to simplify')} <span>{t('your AI integration?')}</span>
          </h2>
          <p className='landing-body-copy mt-5 max-w-xl'>
            {t(
              'Deploy your own gateway and start routing requests through your configured upstream services.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap gap-3 lg:justify-end'>
          <Button
            className='landing-button landing-button-primary'
            render={<Link to='/sign-up' />}
          >
            {t('Get Started')}
            <Icon
              icon={iconifyName('arrowUpRight')}
              width='16'
              height='16'
              aria-hidden='true'
            />
          </Button>
          <Button
            variant='outline'
            className='landing-button landing-button-quiet'
            render={<Link to='/pricing' />}
          >
            {t('View Pricing')}
          </Button>
        </div>
      </AnimateInView>
    </section>
  )
}

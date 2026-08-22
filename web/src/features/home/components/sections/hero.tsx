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

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { iconifyName } from '@/lib/iconify-icons'

import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const signalRows = [
  { label: 'OpenAI', value: '98.4%', tone: 'moss' },
  { label: 'Claude', value: '96.8%', tone: 'ember' },
  { label: 'Gemini', value: '94.2%', tone: 'sand' },
] as const

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const docsUrl =
    (status?.docs_link as string | undefined) || 'https://docs.newapi.pro'

  const docsButton = (
    <Button
      variant='outline'
      className='landing-button landing-button-quiet'
      render={
        docsUrl.startsWith('http') ? (
          <a href={docsUrl} target='_blank' rel='noopener noreferrer' />
        ) : (
          <Link to={docsUrl} />
        )
      }
    >
      <Icon
        icon={iconifyName('book')}
        width='16'
        height='16'
        aria-hidden='true'
      />
      <span>{t('Docs')}</span>
    </Button>
  )

  return (
    <section className='landing-hero relative overflow-hidden px-5 pt-28 pb-16 sm:px-8 md:pt-36 md:pb-24 lg:px-12'>
      <div
        className='landing-grid pointer-events-none absolute inset-0 opacity-60'
        aria-hidden='true'
      />
      <div className='mx-auto grid max-w-[90rem] gap-10 lg:grid-cols-[minmax(0,1.1fr)_minmax(22rem,.9fr)] lg:gap-20'>
        <div className='relative z-10 max-w-3xl'>
          <div className='landing-kicker mb-7'>
            <span className='signal-dot signal-dot-live' />
            <span>{t('AI Application Infrastructure Foundation')}</span>
            <span className='landing-kicker-index'>01 / 04</span>
          </div>

          <h1 className='landing-display max-w-4xl text-[clamp(2.8rem,7vw,7.8rem)]'>
            {t('Unified API Gateway for')}
            <span className='landing-display-accent'>
              {t('Vast Range of AI Models')}
            </span>
          </h1>

          <p className='landing-lede mt-8 max-w-xl'>
            {t(
              'Access a vast selection of models via a standard, unified API protocol. Power AI applications, manage digital assets, and connect the Future.'
            )}
          </p>

          <div className='mt-9 flex flex-wrap items-center gap-3'>
            {props.isAuthenticated ? (
              <Button
                className='landing-button landing-button-primary'
                render={<Link to='/dashboard' />}
              >
                {t('Go to Dashboard')}
                <Icon
                  icon={iconifyName('arrowUpRight')}
                  width='16'
                  height='16'
                  aria-hidden='true'
                />
              </Button>
            ) : (
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
            )}
            {!props.isAuthenticated && (
              <Button
                variant='outline'
                className='landing-button landing-button-quiet'
                render={<Link to='/pricing' />}
              >
                {t('View Pricing')}
              </Button>
            )}
            {docsButton}
          </div>

          <div className='mt-14 grid max-w-2xl grid-cols-[minmax(7rem,.7fr)_minmax(0,1fr)] gap-5 border-t border-[color:var(--border)]/70 pt-5 sm:grid-cols-[9rem_minmax(0,1fr)]'>
            <div className='landing-mono-label'>{t('API')}</div>
            <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-2 text-xs'>
              <span className='inline-flex items-center gap-2'>
                <Icon
                  icon={iconifyName('terminal')}
                  width='15'
                  height='15'
                  aria-hidden='true'
                />
                OpenAI-compatible
              </span>
              <span className='inline-flex items-center gap-2'>
                <Icon
                  icon={iconifyName('route')}
                  width='15'
                  height='15'
                  aria-hidden='true'
                />
                Anthropic / Gemini
              </span>
              <span className='inline-flex items-center gap-2'>
                <Icon
                  icon={iconifyName('layers')}
                  width='15'
                  height='15'
                  aria-hidden='true'
                />
                {t('More Apps')}
              </span>
            </div>
          </div>
        </div>

        <div className='relative z-10 lg:pt-24'>
          <div className='landing-evidence-panel'>
            <div className='flex items-center justify-between border-b border-[color:var(--border)]/80 pb-4'>
              <div className='flex items-center gap-2'>
                <Icon
                  icon={iconifyName('activity')}
                  width='17'
                  height='17'
                  className='text-[color:var(--brand-ember)]'
                  aria-hidden='true'
                />
                <span className='landing-mono-label'>{t('Status')}</span>
              </div>
              <span className='landing-status-chip'>
                <span className='signal-dot signal-dot-live' />
                {t('Healthy')}
              </span>
            </div>

            <div className='text-muted-foreground my-5 flex items-center gap-2 text-[11px]'>
              <span className='landing-route-node'>client</span>
              <span className='landing-route-line' />
              <span className='landing-route-node landing-route-node-active'>
                gateway
              </span>
              <span className='landing-route-line' />
              <span className='landing-route-node'>model</span>
            </div>

            <div className='space-y-2'>
              {signalRows.map((row) => (
                <div key={row.label} className='landing-signal-row'>
                  <span className={`signal-dot signal-dot-${row.tone}`} />
                  <span className='flex-1'>{row.label}</span>
                  <span className='text-muted-foreground font-mono text-[11px]'>
                    {row.value}
                  </span>
                </div>
              ))}
            </div>

            <div className='mt-6 grid grid-cols-2 gap-2 border-t border-[color:var(--border)]/70 pt-4'>
              <div>
                <div className='landing-mono-label'>{t('Latency')}</div>
                <strong className='mt-1 block font-mono text-lg'>182ms</strong>
              </div>
              <div>
                <div className='landing-mono-label'>{t('Cost')}</div>
                <strong className='mt-1 block font-mono text-lg'>
                  $0.0042
                </strong>
              </div>
            </div>
          </div>

          <HeroTerminalDemo className='mt-5' />
        </div>
      </div>
    </section>
  )
}

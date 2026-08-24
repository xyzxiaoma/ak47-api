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
import { Copy } from 'lucide-react'
import { memo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import {
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import { parseTags } from '../lib/filters'
import { isTokenBasedModel } from '../lib/model-helpers'
import { formatPrice, formatRequestPrice } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import { ModelBillingModeBadge } from './model-billing-mode-badge'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  perf?: ModelPerfBadgeData
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const hasCachedPrice = isTokenBased && props.model.cache_ratio != null
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: getDynamicDisplayGroupRatio(
          props.model,
          props.selectedGroup
        ),
      })
    : null

  const primaryGroup = groups[0]
  const bottomTags = [...endpoints.slice(0, 2), ...tags.slice(0, 2)]
  const hiddenCount =
    Math.max(groups.length - 1, 0) +
    Math.max(endpoints.length - 2, 0) +
    Math.max(tags.length - 2, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  let priceSummary: ReactNode
  if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      priceSummary = (
        <span className='min-w-0'>
          <span className='text-amber-700 dark:text-amber-300'>
            {t('Special billing expression')}
          </span>
          <code className='text-muted-foreground/70 mt-0.5 line-clamp-1 block font-mono text-[11px] break-all'>
            {dynamicSummary.rawExpression}
          </code>
        </span>
      )
    } else if (dynamicSummary.primaryEntries.length > 0) {
      priceSummary = (
        <>
          {dynamicSummary.primaryEntries.map((entry) => (
            <span
              key={entry.key}
              className='text-muted-foreground whitespace-nowrap'
            >
              {t(entry.shortLabel)}{' '}
              <span className='text-foreground font-mono font-semibold'>
                {entry.formatted}
              </span>
            </span>
          ))}
        </>
      )
    } else {
      priceSummary = (
        <span className='text-muted-foreground text-sm'>
          {t('Dynamic Pricing')}
        </span>
      )
    }
  } else if (isTokenBased) {
    priceSummary = (
      <>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Input')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'input',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        <span className='text-muted-foreground whitespace-nowrap'>
          {t('Output')}{' '}
          <span className='text-foreground font-mono font-semibold'>
            {formatPrice(
              props.model,
              'output',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
          </span>
        </span>
        {hasCachedPrice && (
          <span className='text-muted-foreground whitespace-nowrap'>
            {t('Cached')}{' '}
            <span className='text-foreground font-mono font-semibold'>
              {formatPrice(
                props.model,
                'cache',
                tokenUnit,
                showRechargePrice,
                priceRate,
                usdExchangeRate,
                props.selectedGroup
              )}
            </span>
          </span>
        )}
      </>
    )
  } else {
    priceSummary = (
      <span className='text-muted-foreground whitespace-nowrap'>
        <span className='text-foreground font-mono font-semibold'>
          {formatRequestPrice(
            props.model,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
        </span>{' '}
        / {t('request')}
      </span>
    )
  }

  return (
    <div
      className={cn(
        'group relative isolate grid min-h-[218px] overflow-visible rounded-[1.1rem] border bg-card p-0 shadow-sm transition-colors',
        'lg:grid-cols-[minmax(0,1.25fr)_minmax(138px,.68fr)]',
        'hover:border-foreground/20 hover:shadow-md'
      )}
    >
      <div className='pointer-events-none absolute inset-[6px_-6px_-6px_6px] -z-10 rounded-[1.1rem] border bg-card [transform:rotate(-1deg)]' />
      <div className='pointer-events-none absolute inset-[3px_-3px_-3px_3px] -z-10 rounded-[1.1rem] border bg-card [transform:rotate(1.2deg)]' />

      <div className='flex min-w-0 flex-col p-3.5'>
        <div className='flex items-start justify-between gap-2'>
          <div className='flex min-w-0 items-start gap-2.5'>
            <div className='bg-muted/40 flex size-9 shrink-0 items-center justify-center rounded-lg'>
              {modelIcon || (
                <span className='text-muted-foreground text-sm font-bold'>
                  {initial}
                </span>
              )}
            </div>
            <div className='min-w-0'>
              <h3 className='text-foreground font-mono text-sm leading-snug font-bold [overflow-wrap:anywhere]'>
                <button
                  type='button'
                  onClick={props.onClick}
                  className='max-w-full text-left hover:underline'
                  title={props.model.model_name}
                >
                  {props.model.model_name}
                </button>
              </h3>
            </div>
          </div>

          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted shrink-0 rounded-lg border p-1.5 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
        </div>

        <div className='mt-5 grid grid-cols-2 gap-x-3 gap-y-1.5 border-t pt-3'>
          <span className='flex flex-col gap-0.5'>
            <small className='text-muted-foreground text-[10px]'>{t('First token')}</small>
            <b className='text-muted-foreground font-mono text-xs'>—</b>
          </span>
          <span className='flex flex-col gap-0.5'>
            <small className='text-muted-foreground text-[10px]'>{t('24 Hours')}</small>
            <b className='text-muted-foreground font-mono text-xs'>—</b>
          </span>
          <span className='flex flex-col gap-0.5'>
            <small className='text-muted-foreground text-[10px]'>{t('Throughput short')}</small>
            <b className='text-muted-foreground font-mono text-xs'>—</b>
          </span>
          <ModelPerfBadge perf={props.perf} className='self-end justify-self-end' />
        </div>

        <div className='mt-3 flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5'>
          {primaryGroup && (
            <span className='text-muted-foreground text-sm font-medium'>
              {primaryGroup}
            </span>
          )}
          <ModelBillingModeBadge model={props.model} />
          {bottomTags.map((item) => (
            <span key={item} className='text-muted-foreground/70 text-xs'>
              {item}
            </span>
          ))}
          <span className='text-muted-foreground/50 text-xs'>{tokenUnitLabel}</span>
          {hiddenCount > 0 && <span className='text-muted-foreground/40 text-xs'>+{hiddenCount}</span>}
        </div>
      </div>

      <aside className='border-border/60 bg-muted/10 flex flex-col rounded-b-[1.1rem] border-t p-3.5 lg:rounded-r-[1.1rem] lg:rounded-bl-none lg:border-t-0 lg:border-l'>
        <div className='flex items-center justify-between gap-2'>
          <strong className='text-foreground text-sm'>{t('Price')}</strong>
          <span className='bg-primary/10 text-primary rounded-md px-1.5 py-1 text-[10px] font-semibold'>
            {primaryGroup || t('Standard')}
          </span>
        </div>
        <div className='mt-3.5 flex flex-col gap-2.5 text-xs'>{priceSummary}</div>
        <div className='text-muted-foreground mt-auto flex items-center justify-between border-t pt-3 text-[10px]'>
          <span>{t('Enabled')}</span>
          <span className='bg-primary/10 text-primary size-2 rounded-full' aria-hidden='true' />
        </div>
      </aside>
    </div>
  )
})

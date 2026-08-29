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
  getDynamicPriceEntries,
  getDynamicDisplayGroupRatio,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import {
  getDisplayGroupRatio,
  getOriginalPricingModel,
  isTokenBasedModel,
} from '../lib/model-helpers'
import { formatPrice, formatRequestPrice } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
  onAdvance?: () => void
  shapeClassName?: string
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const groups = props.model.enable_groups || []
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

  const displayGroupRatio = getDisplayGroupRatio(
    props.model,
    props.selectedGroup
  )
  const hasDiscount = displayGroupRatio < 1
  const discountLabel = `${formatDiscount(displayGroupRatio)}折`
  const primaryGroup = groups[0]
  const originalPriceModel = getOriginalPricingModel(props.model)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  let priceSummary: ReactNode
  if (dynamicSummary) {
    if (dynamicSummary.isSpecialExpression) {
      priceSummary = (
        <span className='min-w-0 text-xs'>
          <span className='text-amber-700 dark:text-amber-300'>
            {t('Special billing expression')}
          </span>
          <code className='text-muted-foreground/70 mt-0.5 line-clamp-1 block font-mono text-[11px] break-all'>
            {dynamicSummary.rawExpression}
          </code>
        </span>
      )
    } else if (dynamicSummary.primaryEntries.length > 0) {
      const originalEntries = getDynamicPriceEntries(dynamicSummary.tier, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: 1,
      })
      priceSummary = (
        <div className='grid gap-2'>
          {dynamicSummary.primaryEntries.map((entry) => {
            const original = originalEntries.find(
              (candidate) => candidate.key === entry.key
            )
            return (
              <PriceRow
                key={entry.key}
                label={t(entry.shortLabel)}
                original={original?.formatted || entry.formatted}
                current={entry.formatted}
                showOriginal={hasDiscount}
              />
            )
          })}
        </div>
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
      <div className='grid gap-2'>
        <PriceRow
          label={t('Input')}
          original={formatPrice(
            originalPriceModel,
            'input',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
          current={formatPrice(
            props.model,
            'input',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
          showOriginal={hasDiscount}
        />
        <PriceRow
          label={t('Output')}
          original={formatPrice(
            originalPriceModel,
            'output',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
          current={formatPrice(
            props.model,
            'output',
            tokenUnit,
            showRechargePrice,
            priceRate,
            usdExchangeRate,
            props.selectedGroup
          )}
          showOriginal={hasDiscount}
        />
        {hasCachedPrice && (
          <PriceRow
            label={t('Cached')}
            original={formatPrice(
              originalPriceModel,
              'cache',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
            current={formatPrice(
              props.model,
              'cache',
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate,
              props.selectedGroup
            )}
            showOriginal={hasDiscount}
          />
        )}
      </div>
    )
  } else {
    priceSummary = (
      <PriceRow
        label={t('request')}
        original={formatRequestPrice(
          originalPriceModel,
          showRechargePrice,
          priceRate,
          usdExchangeRate,
          props.selectedGroup
        )}
        current={formatRequestPrice(
          props.model,
          showRechargePrice,
          priceRate,
          usdExchangeRate,
          props.selectedGroup
        )}
        showOriginal={hasDiscount}
      />
    )
  }

  return (
    <div
      className={cn(
        'group relative isolate overflow-hidden border bg-card/95 p-4 shadow-[0_14px_35px_-28px_hsl(var(--foreground)/0.5)] transition-all',
        props.shapeClassName || 'rounded-[20px]',
        'hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-[0_18px_38px_-24px_hsl(var(--foreground)/0.55)]',
        props.onAdvance && 'cursor-pointer'
      )}
    >
      {props.onAdvance && (
        <button
          type='button'
          aria-label={props.model.model_name}
          onClick={props.onAdvance}
          className={cn(
            'focus-visible:ring-ring absolute inset-0 z-0 focus-visible:ring-2 focus-visible:outline-none',
            props.shapeClassName || 'rounded-2xl'
          )}
        />
      )}
      <div className='pointer-events-none relative z-10 flex min-w-0 flex-col'>
        <div className='flex items-start justify-between gap-2'>
          <div className='flex min-w-0 items-start gap-2.5'>
            <div className='bg-muted/40 flex size-10 shrink-0 items-center justify-center rounded-xl'>
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
                  className='pointer-events-auto max-w-full text-left hover:underline'
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
            className='text-muted-foreground hover:text-foreground hover:bg-muted pointer-events-auto shrink-0 rounded-lg border p-1.5 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
        </div>
        <div className='border-border/70 mt-4 border-t pt-3'>
          <div className='flex items-center justify-between gap-2'>
            <strong className='text-foreground text-sm'>{t('Price')}</strong>
            <div className='flex items-center gap-1.5'>
              {hasDiscount && (
                <span className='bg-primary/10 text-primary rounded-md px-1.5 py-1 text-[10px] font-bold'>
                  {discountLabel}
                </span>
              )}
              <span className='text-muted-foreground bg-muted/60 rounded-md px-1.5 py-1 text-[10px]'>
                {primaryGroup || t('Standard')}
              </span>
            </div>
          </div>
          <div className='mt-2.5 text-xs'>{priceSummary}</div>
        </div>
      </div>
    </div>
  )
})

interface PriceRowProps {
  label: string
  original: string
  current: string
  showOriginal: boolean
}

function PriceRow(props: PriceRowProps) {
  return (
    <div className='flex items-baseline justify-between gap-3'>
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='flex min-w-0 items-baseline justify-end gap-2 text-right'>
        {props.showOriginal && (
          <del className='text-muted-foreground/70 font-mono text-[11px]'>
            {props.original}
          </del>
        )}
        <strong className='text-foreground font-mono font-semibold tabular-nums'>
          {props.current}
        </strong>
      </span>
    </div>
  )
}

function formatDiscount(ratio: number): string {
  const value = Math.max(0, ratio * 10)
  return Number.isInteger(value) ? value.toString() : value.toFixed(1)
}

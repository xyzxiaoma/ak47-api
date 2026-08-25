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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import { DEFAULT_PRICING_PAGE_SIZE, DEFAULT_TOKEN_UNIT } from '../constants'
import type { PricingModel, TokenUnit } from '../types'
import { ModelCard } from './model-card'

export interface ModelCardGridProps {
  models: PricingModel[]
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
}

export function ModelCardGrid(props: ModelCardGridProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = DEFAULT_PRICING_PAGE_SIZE
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const totalPages = Math.max(1, Math.ceil(props.models.length / pageSize))
  const currentPage = Math.min(page, totalPages)

  const pagedModels = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return props.models.slice(start, start + pageSize)
  }, [currentPage, pageSize, props.models])

  const modelGroups = useMemo(() => {
    const groupedModels = new Map<string, PricingModel[]>()

    for (const model of pagedModels) {
      const groupName = model.enable_groups?.[0] || t('Standard')
      const models = groupedModels.get(groupName) || []
      models.push(model)
      groupedModels.set(groupName, models)
    }

    return [...groupedModels.entries()]
  }, [pagedModels, t])

  if (props.models.length === 0) {
    return null
  }

  return (
    <div className='space-y-4 sm:space-y-5'>
      <div className='grid grid-cols-1 items-start gap-4 md:grid-cols-2 xl:grid-cols-4'>
        {modelGroups.map(([groupName, models], index) => (
          <ModelCardStack
            key={groupName}
            groupName={groupName}
            models={models}
            stackIndex={index}
            tokenUnit={tokenUnit}
            priceRate={props.priceRate}
            usdExchangeRate={props.usdExchangeRate}
            showRechargePrice={props.showRechargePrice}
            selectedGroup={props.selectedGroup}
            onModelClick={props.onModelClick}
          />
        ))}
      </div>

      {totalPages > 1 && (
        <div className='text-muted-foreground flex flex-col items-center justify-between gap-3 border-t px-4 py-3 text-sm sm:flex-row'>
          <p className='text-muted-foreground'>
            {t('Page {{current}} of {{total}}', {
              current: currentPage,
              total: totalPages,
            })}
          </p>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => Math.max(1, current - 1))}
              disabled={currentPage <= 1}
              className='gap-1.5'
            >
              <ChevronLeft className='size-4' />
              {t('Previous page')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() =>
                setPage((current) => Math.min(totalPages, current + 1))
              }
              disabled={currentPage >= totalPages}
              className='gap-1.5'
            >
              {t('Next page')}
              <ChevronRight className='size-4' />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

interface ModelCardStackProps {
  groupName: string
  models: PricingModel[]
  stackIndex: number
  onModelClick: (modelName: string) => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit: TokenUnit
  showRechargePrice?: boolean
  selectedGroup?: string
}

const STACK_VARIANTS = [
  {
    front: '[transform:rotate(-1.35deg)_translateY(4px)]',
    backOne: 'left-1 right-[-10px] -top-2 bottom-[-4px] rotate-[2.4deg]',
    backTwo: 'left-[-4px] right-1 -top-1 bottom-[-2px] rotate-[-1.8deg]',
    shape: '[border-radius:24px_18px_22px_20px]',
  },
  {
    front: '[transform:rotate(1.1deg)_translateY(-3px)]',
    backOne: 'left-[-8px] right-1 -top-2 bottom-[-4px] rotate-[-2.6deg]',
    backTwo: 'left-[-3px] right-1 -top-1 bottom-[-2px] rotate-[1.7deg]',
    shape: '[border-radius:18px_24px_20px_22px]',
  },
  {
    front: '[transform:rotate(-0.8deg)_translateY(3px)]',
    backOne: 'left-1 right-[-9px] -top-2 bottom-[-4px] rotate-[2deg]',
    backTwo: 'left-[-4px] right-1 -top-1 bottom-[-2px] rotate-[-2.4deg]',
    shape: '[border-radius:22px_20px_25px_17px]',
  },
  {
    front: '[transform:rotate(1.55deg)_translateY(5px)]',
    backOne: 'left-[-8px] right-1 -top-2 bottom-[-4px] rotate-[-2.1deg]',
    backTwo: 'left-[-3px] right-1 -top-1 bottom-[-2px] rotate-[2.3deg]',
    shape: '[border-radius:20px_26px_18px_23px]',
  },
] as const

function ModelCardStack(props: ModelCardStackProps) {
  const shouldReduceMotion = useReducedMotion()
  const [activeIndex, setActiveIndex] = useState(0)
  const activeModel = props.models[activeIndex % props.models.length]
  const variant = STACK_VARIANTS[props.stackIndex % STACK_VARIANTS.length]

  if (!activeModel) {
    return null
  }

  return (
    <div
      className='relative isolate min-w-0 px-1 pt-1 pb-2 [perspective:1200px]'
      data-model-stack={props.groupName}
      aria-live='polite'
    >
      <div
        aria-hidden
        className={cn(
          'bg-card/75 pointer-events-none absolute z-0 rounded-[20px] border shadow-sm',
          variant.backOne
        )}
      />
      <div
        aria-hidden
        className={cn(
          'bg-card/90 pointer-events-none absolute z-0 rounded-[20px] border shadow-sm',
          variant.backTwo
        )}
      />

      <AnimatePresence initial={false} mode='popLayout'>
        <div className={cn('relative z-10', variant.front)}>
          <motion.div
            key={activeModel.id ?? activeModel.model_name}
            className='relative transform-gpu'
            data-model-stack-card
            initial={
              shouldReduceMotion
                ? { opacity: 1 }
                : { opacity: 0, rotate: -1.5, scale: 0.96, y: 10, zIndex: 5 }
            }
            animate={{
              opacity: 1,
              rotate: 0,
              scale: 1,
              x: 0,
              y: 0,
              zIndex: 10,
            }}
            exit={
              shouldReduceMotion
                ? { opacity: 0 }
                : {
                    opacity: 0,
                    rotate: 7,
                    scale: 0.97,
                    x: 88,
                    y: -18,
                    zIndex: 20,
                  }
            }
            transition={
              shouldReduceMotion
                ? { duration: 0 }
                : { duration: 0.34, ease: [0.22, 1, 0.36, 1] }
            }
          >
            <ModelCard
              model={activeModel}
              tokenUnit={props.tokenUnit}
              priceRate={props.priceRate}
              usdExchangeRate={props.usdExchangeRate}
              showRechargePrice={props.showRechargePrice}
              selectedGroup={props.selectedGroup}
              shapeClassName={variant.shape}
              onClick={() => props.onModelClick(activeModel.model_name || '')}
              onAdvance={
                props.models.length > 1
                  ? () =>
                      setActiveIndex(
                        (current) => (current + 1) % props.models.length
                      )
                  : undefined
              }
            />
          </motion.div>
        </div>
      </AnimatePresence>
    </div>
  )
}

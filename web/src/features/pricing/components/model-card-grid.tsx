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
import { useMemo, useRef, useState } from 'react'
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

interface StackMotionPosition {
  opacity: number
  rotate: number
  scale: number
  x: number
  y: number
  zIndex: number
}

interface StackVariant {
  front: StackMotionPosition
  backOne: StackMotionPosition
  backTwo: StackMotionPosition
  exit: StackMotionPosition
  shape: string
}

const STACK_VARIANTS = [
  {
    front: { opacity: 1, rotate: -1.35, scale: 1, x: 0, y: 4, zIndex: 30 },
    backOne: {
      opacity: 1,
      rotate: 2.4,
      scale: 0.985,
      x: 6,
      y: -4,
      zIndex: 20,
    },
    backTwo: {
      opacity: 0.94,
      rotate: -1.8,
      scale: 0.97,
      x: -4,
      y: -1,
      zIndex: 10,
    },
    exit: {
      opacity: 0,
      rotate: 9,
      scale: 0.96,
      x: 104,
      y: -30,
      zIndex: 40,
    },
    shape: '[border-radius:24px_18px_22px_20px]',
  },
  {
    front: { opacity: 1, rotate: 1.1, scale: 1, x: 0, y: -3, zIndex: 30 },
    backOne: {
      opacity: 1,
      rotate: -2.6,
      scale: 0.985,
      x: -6,
      y: -4,
      zIndex: 20,
    },
    backTwo: {
      opacity: 0.94,
      rotate: 1.7,
      scale: 0.97,
      x: -3,
      y: -1,
      zIndex: 10,
    },
    exit: {
      opacity: 0,
      rotate: -9,
      scale: 0.96,
      x: -104,
      y: -30,
      zIndex: 40,
    },
    shape: '[border-radius:18px_24px_20px_22px]',
  },
  {
    front: { opacity: 1, rotate: -0.8, scale: 1, x: 0, y: 3, zIndex: 30 },
    backOne: {
      opacity: 1,
      rotate: 2,
      scale: 0.985,
      x: 6,
      y: -4,
      zIndex: 20,
    },
    backTwo: {
      opacity: 0.94,
      rotate: -2.4,
      scale: 0.97,
      x: -4,
      y: -1,
      zIndex: 10,
    },
    exit: {
      opacity: 0,
      rotate: 8,
      scale: 0.96,
      x: 104,
      y: -30,
      zIndex: 40,
    },
    shape: '[border-radius:22px_20px_25px_17px]',
  },
  {
    front: { opacity: 1, rotate: 1.55, scale: 1, x: 0, y: 5, zIndex: 30 },
    backOne: {
      opacity: 1,
      rotate: -2.1,
      scale: 0.985,
      x: -6,
      y: -4,
      zIndex: 20,
    },
    backTwo: {
      opacity: 0.94,
      rotate: 2.3,
      scale: 0.97,
      x: -3,
      y: -1,
      zIndex: 10,
    },
    exit: {
      opacity: 0,
      rotate: -10,
      scale: 0.96,
      x: -104,
      y: -30,
      zIndex: 40,
    },
    shape: '[border-radius:20px_26px_18px_23px]',
  },
] satisfies readonly StackVariant[]

function ModelCardStack(props: ModelCardStackProps) {
  const shouldReduceMotion = useReducedMotion()
  const [activeIndex, setActiveIndex] = useState(0)
  const [isAnimating, setIsAnimating] = useState(false)
  const animationLockRef = useRef(false)
  const activeModel = props.models[activeIndex % props.models.length]
  const variant = STACK_VARIANTS[props.stackIndex % STACK_VARIANTS.length]

  if (!activeModel) {
    return null
  }

  const layerCount = Math.min(props.models.length, isAnimating ? 4 : 3)
  const visibleLayers = Array.from({ length: layerCount }, (_, depth) => ({
    depth,
    model: props.models[(activeIndex + depth) % props.models.length],
  }))

  const advanceStack = () => {
    if (animationLockRef.current || props.models.length <= 1) {
      return
    }

    if (shouldReduceMotion) {
      setActiveIndex((current) => (current + 1) % props.models.length)
      return
    }

    animationLockRef.current = true
    setIsAnimating(true)
  }

  const finishAdvance = () => {
    if (!animationLockRef.current) {
      return
    }

    setActiveIndex((current) => (current + 1) % props.models.length)
    setIsAnimating(false)
    animationLockRef.current = false
  }

  return (
    <div
      className='relative isolate min-w-0 px-1 pt-1 pb-2 [perspective:1200px]'
      data-model-stack={props.groupName}
      aria-live='polite'
      aria-busy={isAnimating}
    >
      <AnimatePresence initial={false}>
        {visibleLayers.map(({ depth, model }) => {
          const isFront = depth === 0
          const isLayerInert = isAnimating || !isFront
          let target = variant.front

          if (isAnimating) {
            if (depth === 0) {
              target = variant.exit
            } else if (depth === 1) {
              target = variant.front
            } else if (depth === 2) {
              target = variant.backOne
            } else {
              target = variant.backTwo
            }
          } else if (depth === 1) {
            target = variant.backOne
          } else if (depth === 2) {
            target = variant.backTwo
          }

          return (
            <motion.div
              key={model.id ?? model.model_name}
              className={cn(
                'left-1 right-1 transform-gpu',
                isFront ? 'relative' : 'pointer-events-none absolute top-1'
              )}
              data-model-stack-card={isFront ? '' : undefined}
              aria-hidden={isLayerInert}
              inert={isLayerInert}
              initial={
                shouldReduceMotion || depth < 3
                  ? false
                  : {
                      ...variant.backTwo,
                      opacity: 0,
                      scale: variant.backTwo.scale - 0.02,
                    }
              }
              animate={target}
              exit={{ opacity: 0 }}
              transition={
                shouldReduceMotion
                  ? { duration: 0 }
                  : {
                      duration: depth === 0 ? 0.38 : 0.32,
                      ease: [0.22, 1, 0.36, 1],
                    }
              }
              onAnimationComplete={
                isAnimating && depth === 0 ? finishAdvance : undefined
              }
            >
              <ModelCard
                model={model}
                tokenUnit={props.tokenUnit}
                priceRate={props.priceRate}
                usdExchangeRate={props.usdExchangeRate}
                showRechargePrice={props.showRechargePrice}
                selectedGroup={props.selectedGroup}
                shapeClassName={variant.shape}
                onClick={() => props.onModelClick(model.model_name || '')}
                onAdvance={
                  isFront && !isAnimating && props.models.length > 1
                    ? advanceStack
                    : undefined
                }
              />
            </motion.div>
          )
        })}
      </AnimatePresence>
    </div>
  )
}

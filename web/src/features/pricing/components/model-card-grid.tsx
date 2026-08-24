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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

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
      {modelGroups.map(([groupName, models]) => (
        <section key={groupName} className='space-y-3'>
          <header className='flex items-end justify-between border-b pb-2'>
            <div>
              <h2 className='text-foreground text-base font-semibold'>
                {groupName}
              </h2>
              <p className='text-muted-foreground text-xs'>
                {t('{{count}} models', { count: models.length })}
              </p>
            </div>
            <span className='bg-muted text-muted-foreground rounded-full px-2 py-1 text-xs font-medium'>
              {models.length}
            </span>
          </header>

          <div className='grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3'>
            {models.map((model) => (
              <ModelCard
                key={model.id ?? model.model_name}
                model={model}
                tokenUnit={tokenUnit}
                priceRate={props.priceRate}
                usdExchangeRate={props.usdExchangeRate}
                showRechargePrice={props.showRechargePrice}
                selectedGroup={props.selectedGroup}
                onClick={() => props.onModelClick(model.model_name || '')}
              />
            ))}
          </div>
        </section>
      ))}

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

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
import { useRef, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

interface CounterProps {
  end: number
  suffix?: string
  prefix?: string
  duration?: number
  decimals?: number
}

function Counter(props: CounterProps) {
  const { end, suffix = '', prefix = '', duration = 1600, decimals = 0 } = props
  const ref = useRef<HTMLSpanElement>(null)
  const startedRef = useRef(false)

  const formatValue = useCallback(
    (v: number) =>
      decimals > 0 ? v.toFixed(decimals) : Math.round(v).toLocaleString(),
    [decimals]
  )

  const animate = useCallback(() => {
    const el = ref.current
    if (!el) return
    const start = performance.now()
    const step = (now: number) => {
      const progress = Math.min((now - start) / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      el.textContent = `${prefix}${formatValue(eased * end)}${suffix}`
      if (progress < 1) requestAnimationFrame(step)
    }
    requestAnimationFrame(step)
  }, [end, duration, prefix, suffix, formatValue])

  useEffect(() => {
    const el = ref.current
    if (!el) return

    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    if (mq.matches) {
      el.textContent = `${prefix}${formatValue(end)}${suffix}`
      return
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !startedRef.current) {
          startedRef.current = true
          animate()
          observer.unobserve(el)
        }
      },
      { threshold: 0.5 }
    )

    observer.observe(el)
    return () => observer.disconnect()
  }, [animate, end, prefix, suffix, formatValue])

  return (
    <span ref={ref} className='tabular-nums'>
      {prefix}0{suffix}
    </span>
  )
}

interface StatsProps {
  className?: string
}

interface StatItem {
  end: number
  suffix: string
  label: string
  decimals?: number
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const stats: StatItem[] = [
    { end: 50, suffix: '+', label: t('upstream services integrated') },
    { end: 100, suffix: '+', label: t('model billing support') },
    { end: 50, suffix: '+', label: t('compatible API routes') },
    { end: 10, suffix: '+', label: t('scheduling controls') },
  ]

  return (
    <div className='landing-stats relative z-10 border-y border-[color:var(--border)]/80'>
      <div className='mx-auto grid max-w-[90rem] gap-8 px-5 py-8 sm:px-8 md:grid-cols-[.8fr_1fr_1fr_1.2fr] md:gap-0 md:py-10 lg:px-12'>
        {stats.map((s) => (
          <div
            key={s.label}
            className='flex items-start gap-4 border-[color:var(--border)]/70 md:border-l md:px-7 first:md:border-l-0 first:md:pl-0'
          >
            <span className='font-mono text-2xl font-semibold tracking-tight md:text-3xl'>
              <Counter end={s.end} suffix={s.suffix} decimals={s.decimals} />
            </span>
            <span className='landing-mono-label max-w-28 pt-1 text-[10px] leading-relaxed'>
              {s.label}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

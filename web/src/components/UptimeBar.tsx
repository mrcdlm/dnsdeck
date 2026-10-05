import { useTranslation } from 'react-i18next'

import type { Uptime } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { bucketColor, bucketKey, formatPercent } from '@/lib/tunnels'
import { cn } from '@/lib/utils'

interface Props {
  uptime: Uptime
  label: string
  observedSince?: string
  /** Texte der Abschnitte (Tooltip): tunnel.bucket.* oder checks.bucket.* */
  bucketTexts?: 'tunnel.bucket' | 'checks.bucket'
  /** Schmale Variante für Tabellen: Prozent neben dem Balken, ohne Überschrift */
  compact?: boolean
}

export function UptimeBar({ uptime, label, observedSince, bucketTexts = 'tunnel.bucket', compact }: Props) {
  const { t } = useTranslation()
  const now = useNow()
  const start = new Date(uptime.start).getTime()
  const step = uptime.bucket_seconds * 1000
  const title = t('tunnels.uptime', { range: label })

  const bar = (
    <div
      className={cn('flex gap-px', compact ? 'h-4 min-w-24 flex-1' : 'h-6')}
      role="img"
      aria-label={`${title}: ${formatPercent(uptime.percent)}`}
    >
      {uptime.buckets.map((b, i) => {
        const from = new Date(start + i * step).toISOString()
        const to = new Date(start + (i + 1) * step).toISOString()
        return (
          <div
            key={i}
            className={cn('min-w-0 flex-1', compact ? 'rounded-[1px]' : 'rounded-[2px]', bucketColor[b])}
            title={`${absoluteTime(from)} – ${absoluteTime(to)}: ${t(`${bucketTexts}.${bucketKey(b)}`)}`}
          />
        )
      })}
    </div>
  )

  if (compact) {
    return (
      <div className="flex items-center gap-2">
        {bar}
        <span className="w-14 shrink-0 text-right text-xs font-medium tabular-nums">{formatPercent(uptime.percent)}</span>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground">{title}</span>
        <span className="font-medium tabular-nums">{formatPercent(uptime.percent)}</span>
      </div>
      {bar}
      {uptime.observed < 0.99 &&
        (observedSince && new Date(observedSince).getTime() > start ? (
          <span className="text-muted-foreground text-xs" title={absoluteTime(observedSince)}>
            {t('tunnels.observedSince', { time: relativeTime(observedSince, now) })}
          </span>
        ) : (
          <span className="text-muted-foreground text-xs">{t('tunnels.gaps')}</span>
        ))}
    </div>
  )
}

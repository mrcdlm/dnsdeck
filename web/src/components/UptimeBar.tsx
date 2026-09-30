import { useTranslation } from 'react-i18next'

import type { Uptime } from '@/lib/api'
import { absoluteTime, relativeTime, useNow } from '@/lib/format'
import { bucketColor, bucketKey, formatPercent } from '@/lib/tunnels'
import { cn } from '@/lib/utils'

export function UptimeBar({ uptime, label, observedSince }: { uptime: Uptime; label: string; observedSince?: string }) {
  const { t } = useTranslation()
  const now = useNow()
  const start = new Date(uptime.start).getTime()
  const step = uptime.bucket_seconds * 1000
  const title = t('tunnels.uptime', { range: label })

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between text-xs">
        <span className="text-muted-foreground">{title}</span>
        <span className="font-medium tabular-nums">{formatPercent(uptime.percent)}</span>
      </div>
      <div className="flex h-6 gap-px" role="img" aria-label={`${title}: ${formatPercent(uptime.percent)}`}>
        {uptime.buckets.map((b, i) => {
          const from = new Date(start + i * step).toISOString()
          const to = new Date(start + (i + 1) * step).toISOString()
          return (
            <div
              key={i}
              className={cn('min-w-0 flex-1 rounded-[2px]', bucketColor[b])}
              title={`${absoluteTime(from)} – ${absoluteTime(to)}: ${t(`tunnel.bucket.${bucketKey(b)}`)}`}
            />
          )
        })}
      </div>
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
